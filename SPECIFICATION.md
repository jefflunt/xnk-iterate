# Technical Specification (`xnk-capture`)

This document defines the data models, wire protocols, file formats, and hardware configuration specifications for `xnk-capture`.

---

## 1. Capture Formats & Encoding Specifications

### 1.1 Video Specification
* **Capture Framework:** `ScreenCaptureKit` (`SCStream`)
* **Codec:** Apple VideoToolbox Hardware `H.264` (Main Profile) or `HEVC` (H.265)
* **Container Format:** Fragmented MP4 (`fMP4`) with 1-second segment intervals
* **Target Frame Rate:** 60 FPS (with automatic 30 FPS fallback on battery or low-power state)
* **Pixel Format:** `kCVPixelFormatType_32BGRA` or `kCVPixelFormatType_420YpCbCr8BiPlanarVideoRange`
* **Resolution Modes:**
  * **Native Retina (1:1):** Pixel-perfect UI capture for crisp typography and small details.
  * **Standard 1080p (Scale to Fit):** Bandwidth-saving mode for full-screen recording.

### 1.2 Audio Specification
* **Input Device:** Default system input device (`AVAudioEngine.inputNode`)
* **Format:** Linear PCM 48.0 kHz, 16-bit Mono (or AAC-LC 128 kbps in container)
* **Voice Activity Detection (VAD) Thresholds:**
  * **Pre-speech Padding:** 200 ms
  * **Silence Termination Threshold:** 400 ms – 500 ms continuous silence to mark end-of-utterance.

---

## 2. Data Models (Swift Representation)

```swift
import Foundation
import CoreGraphics

/// Root metadata bundle written to manifest.json
public struct CaptureManifest: Codable {
    public let captureId: UUID
    public let createdAt: Date
    public let durationSeconds: Double
    public let target: TargetMetadata
    public let videoFile: String
    public let utterances: [Utterance]
    public let interactions: [InteractionEvent]
    public let frames: [FrameSnapshot]
}

/// Metadata about the captured screen or window
public struct TargetMetadata: Codable {
    public let type: TargetType // .window, .display, .app
    public let title: String
    public let bundleIdentifier: String?
    public let resolution: CGSize
}

public enum TargetType: String, Codable {
    case display
    case window
    case application
}

/// A finalized director speech comment
public struct Utterance: Codable {
    public let id: Int
    public let transcript: String
    public let confidence: Float
    public let speechStart: Double
    public let speechEnd: Double
    public let lookbackWindow: LookbackRange
    public let triggerAction: InteractionEvent?
    public let triggerFramePath: String
    public let reactionFramePath: String
}

public struct LookbackRange: Codable {
    public let startSeconds: Double
    public let endSeconds: Double
    public let triggerSeconds: Double
    public let duration: Double
}

/// An OS-level user interaction captured via CGEventTap
public struct InteractionEvent: Codable {
    public let timestamp: Double
    public let type: InteractionType
    public let location: CGPoint?
    public let keyCommand: String?
    public let focusedApp: String?
}

public enum InteractionType: String, Codable {
    case mouseDown
    case mouseUp
    case rightClick
    case keyPress
    case hotkey
    case appSwitch
}

/// An extracted visual screenshot
public struct FrameSnapshot: Codable {
    public let timestamp: Double
    public let relativePath: String
    public let role: FrameRole // .trigger, .reaction, .carousel
    public let utteranceId: Int
}

public enum FrameRole: String, Codable {
    case trigger
    case reaction
    case carousel
}
```

---

## 3. Real-Time WebSocket Protocol

When running in live mode, `xnk-capture` hosts a WebSocket server at `ws://127.0.0.1:9292`.

### 3.1 Event: `capture_started`
Emitted immediately when recording begins.
```json
{
  "event": "capture_started",
  "capture_id": "8f3b6c2d-98e1-4c91-9e23-7fa34710bc4e",
  "timestamp": 1795281000.123,
  "target": {
    "type": "window",
    "title": "King's Claim — Safari",
    "bundle_id": "com.apple.Safari",
    "resolution": { "width": 1440, "height": 900 }
  }
}
```

### 3.2 Event: `director_utterance`
Emitted the instant speech silence terminates a comment.
```json
{
  "event": "director_utterance",
  "capture_id": "8f3b6c2d-98e1-4c91-9e23-7fa34710bc4e",
  "utterance": {
    "id": 4,
    "transcript": "The scoreboard colors are inverted. Player 1 is blue on the board but red in the score box.",
    "confidence": 0.963,
    "speech_start": 28.50,
    "speech_end": 33.20,
    "lookback": {
      "start": 24.50,
      "end": 33.20,
      "trigger_timestamp": 25.80,
      "duration": 8.70
    },
    "trigger_action": {
      "type": "mouseDown",
      "timestamp": 25.80,
      "location": { "x": 1380, "y": 42 },
      "focused_app": "com.apple.Safari"
    },
    "trigger_frame_path": "/Users/jefflunt/code/xnk-capture/storage/recordings/frames/frame_04_trigger_25.80s.png",
    "reaction_frame_path": "/Users/jefflunt/code/xnk-capture/storage/recordings/frames/frame_04_reaction_33.20s.png"
  }
}
```

### 3.3 Event: `capture_completed`
Emitted when recording stops and the archive package is sealed.
```json
{
  "event": "capture_completed",
  "capture_id": "8f3b6c2d-98e1-4c91-9e23-7fa34710bc4e",
  "duration_seconds": 145.2,
  "total_utterances": 18,
  "bundle_path": "/Users/jefflunt/code/xnk-capture/storage/recordings/capture_8f3b6c2d_1795281000/"
}
```

---

## 4. macOS Permissions & System Capabilities Required

1. **Screen Recording:** `CGRequestScreenCaptureAccess()` (Required for `ScreenCaptureKit` and window surface access).
2. **Microphone Access:** `AVCaptureDevice.requestAccess(for: .audio)` (Required for speech ingestion).
3. **Speech Recognition:** `SFSpeechRecognizer.requestAuthorization()` (Required for Apple offline/online speech transcription).
4. **Accessibility API (`AXUIElement`) & Input Monitoring:** Required for `CGEventTap` to track mouse clicks and keyboard events across other applications.
