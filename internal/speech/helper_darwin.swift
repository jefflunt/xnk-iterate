import Foundation
import AVFoundation
import Speech

// Native macOS speech + VAD helper (Voice-Activated Start + Explicit Hotkey Flush/Discard).
// Accumulates transcript continuously across pauses until Go writes "flush\n" or "discard\n" to stdin.

struct SpeechJSONEvent: Encodable {
    let type: String
    let level: Double?
    let text: String?
    let confidence: Float?
    let silence_offset: Double?
    let message: String?
}

func emit(_ event: SpeechJSONEvent) {
    let encoder = JSONEncoder()
    if let data = try? encoder.encode(event), let str = String(data: data, encoding: .utf8) {
        print(str)
        fflush(stdout)
    }
}

final class LiveSpeechRunner {
    private let audioEngine = AVAudioEngine()
    private let speechRecognizer = SFSpeechRecognizer(locale: Locale(identifier: "en-US"))
    private let requestLock = NSLock()
    private var recognitionRequest: SFSpeechAudioBufferRecognitionRequest?
    private var recognitionTask: SFSpeechRecognitionTask?
    private var taskGeneration: Int = 0

    private var turnActive = false
    private var speechActiveSince: Date?
    private var lastVoiceAt: Date?
    private var committedSegments: [String] = []
    private var currentSegment = ""
    private var lastConfidence: Float = 0.92
    private var loggedMicAudio = false
    private var lastErrorLogAt: Date = .distantPast

    private let vadThreshold: Float = 0.02
    private let minSpeechDuration: TimeInterval = 0.15

    func start() {
        startStdinCommandLoop()

        SFSpeechRecognizer.requestAuthorization { authStatus in
            DispatchQueue.main.async {
                switch authStatus {
                case .authorized:
                    let avail = self.speechRecognizer?.isAvailable ?? false
                    let onDev = self.speechRecognizer?.supportsOnDeviceRecognition ?? false
                    emit(SpeechJSONEvent(type: "status", level: nil, text: nil, confidence: nil, silence_offset: nil, message: "speech_authorized (available=\(avail) onDevice=\(onDev))"))
                    self.startAudioAndRecognition()
                default:
                    emit(SpeechJSONEvent(type: "error", level: nil, text: nil, confidence: nil, silence_offset: nil, message: "speech_permission_denied (status=\(authStatus.rawValue))"))
                }
            }
        }
    }

    private func startStdinCommandLoop() {
        DispatchQueue.global(qos: .userInteractive).async { [weak self] in
            var buffer = Data()
            var byte: UInt8 = 0
            while true {
                let n = read(STDIN_FILENO, &byte, 1)
                if n <= 0 {
                    break
                }
                if byte == 10 { // '\n'
                    if let line = String(data: buffer, encoding: .utf8) {
                        let cmd = line.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
                        DispatchQueue.main.async {
                            guard let self = self else { return }
                            if cmd == "flush" {
                                self.flushTurn()
                            } else if cmd == "discard" {
                                self.discardTurn()
                            }
                        }
                    }
                    buffer.removeAll(keepingCapacity: true)
                } else {
                    buffer.append(byte)
                }
            }
        }
    }

    private func fullTranscript() -> String {
        var parts = committedSegments
        let cur = currentSegment.trimmingCharacters(in: .whitespacesAndNewlines)
        if !cur.isEmpty {
            parts.append(cur)
        }
        return parts.joined(separator: " ").trimmingCharacters(in: .whitespacesAndNewlines)
    }

    private func setActiveRequest(_ req: SFSpeechAudioBufferRecognitionRequest?) {
        requestLock.lock()
        recognitionRequest = req
        requestLock.unlock()
    }

    private func appendAudioBuffer(_ buffer: AVAudioPCMBuffer) {
        requestLock.lock()
        let req = recognitionRequest
        requestLock.unlock()
        req?.append(buffer)
    }

    private func startAudioAndRecognition() {
        let inputNode = audioEngine.inputNode
        let hwFormat = inputNode.outputFormat(forBus: 0)
        emit(SpeechJSONEvent(type: "status", level: nil, text: nil, confidence: nil, silence_offset: nil, message: "audio_format sampleRate=\(hwFormat.sampleRate) channels=\(hwFormat.channelCount)"))

        startNewRecognitionTask()

        // Passing format: nil uses the native hardware input format without VoiceProcessingIO
        // (which can output silence or incompatible channel layouts on macOS when no output node is active).
        inputNode.installTap(onBus: 0, bufferSize: 1024, format: nil) { [weak self] buffer, _ in
            guard let self = self else { return }
            self.appendAudioBuffer(buffer)
            let rms = self.computeRMS(buffer: buffer)
            let normalized = min(1.0, Double(rms * 8.0))
            emit(SpeechJSONEvent(type: "audio_level", level: normalized, text: nil, confidence: nil, silence_offset: nil, message: nil))

            DispatchQueue.main.async {
                let now = Date()
                if rms > 0.005 && !self.loggedMicAudio {
                    self.loggedMicAudio = true
                    emit(SpeechJSONEvent(type: "status", level: nil, text: nil, confidence: nil, silence_offset: nil, message: String(format: "mic_audio_detected rms=%.4f", rms)))
                }
                if rms > self.vadThreshold {
                    self.lastVoiceAt = now
                    if self.speechActiveSince == nil {
                        self.speechActiveSince = now
                    }
                    if !self.turnActive, let started = self.speechActiveSince, now.timeIntervalSince(started) >= self.minSpeechDuration {
                        self.turnActive = true
                        emit(SpeechJSONEvent(type: "vad_start", level: normalized, text: nil, confidence: nil, silence_offset: nil, message: nil))
                    }
                } else if !self.turnActive {
                    self.speechActiveSince = nil
                }
            }
        }

        audioEngine.prepare()
        do {
            try audioEngine.start()
            emit(SpeechJSONEvent(type: "status", level: nil, text: nil, confidence: nil, silence_offset: nil, message: "audio_running"))
        } catch {
            emit(SpeechJSONEvent(type: "error", level: nil, text: nil, confidence: nil, silence_offset: nil, message: "audio_start_failed: \(error.localizedDescription)"))
        }
    }

    private func startNewRecognitionTask() {
        taskGeneration += 1
        let gen = taskGeneration

        recognitionTask?.cancel()
        recognitionTask = nil

        let request = SFSpeechAudioBufferRecognitionRequest()
        request.shouldReportPartialResults = true
        if #available(macOS 13.0, *) {
            request.addsPunctuation = true
        }
        request.requiresOnDeviceRecognition = false
        setActiveRequest(request)

        recognitionTask = speechRecognizer?.recognitionTask(with: request) { [weak self] result, error in
            DispatchQueue.main.async {
                guard let self = self, self.taskGeneration == gen else { return }
                if let result = result {
                    let text = result.bestTranscription.formattedString.trimmingCharacters(in: .whitespacesAndNewlines)
                    if !text.isEmpty {
                        self.currentSegment = text
                        if let seg = result.bestTranscription.segments.last, seg.confidence > 0 {
                            self.lastConfidence = seg.confidence
                        }
                        if !self.turnActive {
                            self.turnActive = true
                            emit(SpeechJSONEvent(type: "vad_start", level: nil, text: nil, confidence: nil, silence_offset: nil, message: nil))
                        }
                        emit(SpeechJSONEvent(type: "partial", level: nil, text: self.fullTranscript(), confidence: self.lastConfidence, silence_offset: nil, message: nil))
                    }
                    if result.isFinal {
                        // Apple's recognizer hit its segment limit — commit segment and keep listening seamlessly!
                        self.commitCurrentSegmentAndRestart()
                    }
                } else if let error = error {
                    let now = Date()
                    if now.timeIntervalSince(self.lastErrorLogAt) >= 3.0 {
                        self.lastErrorLogAt = now
                        emit(SpeechJSONEvent(type: "status", level: nil, text: nil, confidence: nil, silence_offset: nil, message: "recognition_error: \(error.localizedDescription)"))
                    }
                    self.commitCurrentSegmentAndRestart()
                }
            }
        }
    }

    private func commitCurrentSegmentAndRestart() {
        let cur = currentSegment.trimmingCharacters(in: .whitespacesAndNewlines)
        if !cur.isEmpty {
            committedSegments.append(cur)
            currentSegment = ""
        }
        setActiveRequest(nil)
        recognitionTask?.cancel()
        recognitionTask = nil
        DispatchQueue.main.asyncAfter(deadline: .now() + 0.3) { [weak self] in
            self?.startNewRecognitionTask()
        }
    }

    private func flushTurn() {
        taskGeneration += 1
        let text = fullTranscript()
        let silenceOffset = lastVoiceAt != nil ? Date().timeIntervalSince(lastVoiceAt!) : 0.0
        turnActive = false
        speechActiveSince = nil
        lastVoiceAt = nil
        committedSegments.removeAll()
        currentSegment = ""

        setActiveRequest(nil)
        recognitionTask?.cancel()
        recognitionTask = nil

        if !text.isEmpty {
            emit(SpeechJSONEvent(type: "final", level: nil, text: text, confidence: lastConfidence, silence_offset: silenceOffset, message: nil))
        }
        emit(SpeechJSONEvent(type: "vad_end", level: nil, text: nil, confidence: nil, silence_offset: nil, message: nil))
        DispatchQueue.main.asyncAfter(deadline: .now() + 0.05) { [weak self] in
            self?.startNewRecognitionTask()
        }
    }

    private func discardTurn() {
        taskGeneration += 1
        turnActive = false
        speechActiveSince = nil
        lastVoiceAt = nil
        committedSegments.removeAll()
        currentSegment = ""

        setActiveRequest(nil)
        recognitionTask?.cancel()
        recognitionTask = nil

        emit(SpeechJSONEvent(type: "vad_end", level: nil, text: nil, confidence: nil, silence_offset: nil, message: nil))
        DispatchQueue.main.asyncAfter(deadline: .now() + 0.05) { [weak self] in
            self?.startNewRecognitionTask()
        }
    }

    private func computeRMS(buffer: AVAudioPCMBuffer) -> Float {
        guard let channelData = buffer.floatChannelData?[0] else { return 0 }
        let frames = Int(buffer.frameLength)
        if frames == 0 { return 0 }
        var sum: Float = 0
        for i in 0..<frames {
            let s = channelData[i]
            sum += s * s
        }
        return sqrt(sum / Float(frames))
    }
}

let runner = LiveSpeechRunner()
runner.start()
RunLoop.main.run()
