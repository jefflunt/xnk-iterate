import Foundation
import AVFoundation
import Speech

// Native macOS speech + VAD helper with hardware Acoustic Echo Cancellation (AEC).
// Emits newline-delimited JSON events to stdout for the Go host process.

struct SpeechJSONEvent: Encodable {
    let type: String
    let level: Double?
    let text: String?
    let confidence: Float?
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
    private var recognitionRequest: SFSpeechAudioBufferRecognitionRequest?
    private var recognitionTask: SFSpeechRecognitionTask?

    private var isSpeaking = false
    private var speechActiveSince: Date?
    private var lastVoiceAt: Date?
    private var currentTranscript = ""
    private var lastConfidence: Float = 0.92
    private var silenceTimer: Timer?

    private let vadThreshold: Float = 0.025
    private let minSpeechDuration: TimeInterval = 0.20
    private let silenceTimeout: TimeInterval = 0.48

    func start() {
        SFSpeechRecognizer.requestAuthorization { authStatus in
            DispatchQueue.main.async {
                switch authStatus {
                case .authorized:
                    emit(SpeechJSONEvent(type: "status", level: nil, text: nil, confidence: nil, message: "speech_authorized"))
                    self.startAudioAndRecognition()
                default:
                    emit(SpeechJSONEvent(type: "error", level: nil, text: nil, confidence: nil, message: "speech_permission_denied"))
                }
            }
        }
    }

    private func startAudioAndRecognition() {
        let inputNode = audioEngine.inputNode
        // Enable hardware Voice Processing I/O for Acoustic Echo Cancellation (AEC) so agent TTS does not re-trigger mic STT.
        try? inputNode.setVoiceProcessingEnabled(true)

        let format = inputNode.outputFormat(forBus: 0)
        startNewRecognitionTask()

        inputNode.installTap(onBus: 0, bufferSize: 1024, format: format) { [weak self] buffer, _ in
            guard let self = self else { return }
            self.recognitionRequest?.append(buffer)
            let rms = self.computeRMS(buffer: buffer)
            let normalized = min(1.0, Double(rms * 8.0))
            emit(SpeechJSONEvent(type: "audio_level", level: normalized, text: nil, confidence: nil, message: nil))

            let now = Date()
            if rms > self.vadThreshold {
                self.lastVoiceAt = now
                if self.speechActiveSince == nil {
                    self.speechActiveSince = now
                }
                if !self.isSpeaking, let started = self.speechActiveSince, now.timeIntervalSince(started) >= self.minSpeechDuration {
                    self.isSpeaking = true
                    emit(SpeechJSONEvent(type: "vad_start", level: normalized, text: nil, confidence: nil, message: nil))
                }
            } else {
                if !self.isSpeaking {
                    self.speechActiveSince = nil
                } else if let last = self.lastVoiceAt, now.timeIntervalSince(last) >= self.silenceTimeout {
                    self.finalizeUtterance()
                }
            }
        }

        audioEngine.prepare()
        do {
            try audioEngine.start()
            emit(SpeechJSONEvent(type: "status", level: nil, text: nil, confidence: nil, message: "audio_running"))
        } catch {
            emit(SpeechJSONEvent(type: "error", level: nil, text: nil, confidence: nil, message: "audio_start_failed: \(error.localizedDescription)"))
        }
    }

    private func startNewRecognitionTask() {
        recognitionTask?.cancel()
        recognitionTask = nil

        let request = SFSpeechAudioBufferRecognitionRequest()
        request.shouldReportPartialResults = true
        if #available(macOS 13.0, *) {
            request.addsPunctuation = true
        }
        if let recognizer = speechRecognizer, recognizer.supportsOnDeviceRecognition {
            request.requiresOnDeviceRecognition = true
        }
        recognitionRequest = request

        recognitionTask = speechRecognizer?.recognitionTask(with: request) { [weak self] result, error in
            guard let self = self else { return }
            if let result = result {
                let text = result.bestTranscription.formattedString.trimmingCharacters(in: .whitespacesAndNewlines)
                if !text.isEmpty {
                    self.currentTranscript = text
                    if let seg = result.bestTranscription.segments.last, seg.confidence > 0 {
                        self.lastConfidence = seg.confidence
                    }
                    emit(SpeechJSONEvent(type: "partial", level: nil, text: text, confidence: self.lastConfidence, message: nil))
                }
                if result.isFinal {
                    self.finalizeUtterance()
                }
            }
            if error != nil {
                self.restartRecognitionRequest()
            }
        }
    }

    private func finalizeUtterance() {
        guard isSpeaking || !currentTranscript.isEmpty else { return }
        let finalText = currentTranscript.trimmingCharacters(in: .whitespacesAndNewlines)
        isSpeaking = false
        speechActiveSince = nil
        lastVoiceAt = nil
        currentTranscript = ""

        if !finalText.isEmpty {
            emit(SpeechJSONEvent(type: "final", level: nil, text: finalText, confidence: lastConfidence, message: nil))
        }
        emit(SpeechJSONEvent(type: "vad_end", level: nil, text: nil, confidence: nil, message: nil))
        restartRecognitionRequest()
    }

    private func restartRecognitionRequest() {
        recognitionRequest?.endAudio()
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
