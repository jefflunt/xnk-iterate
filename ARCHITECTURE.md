# System Architecture (`xnk-capture`)

This document outlines the architectural design of `xnk-capture`, detailing how native macOS frameworks, real-time audio/video processing, temporal indexing, and AI agent integration work together.

---

## 1. High-Level Architecture Diagram

```mermaid
flowchart TD
    subgraph macOS Subsystems
        SCK["ScreenCaptureKit (SCStream)\nGPU Surfaces @ 60 FPS"]
        MIC["AVAudioEngine / CoreAudio\n48kHz Mono Microphone Input"]
        EVENTS["CGEventTap & Accessibility APIs\nMouse Clicks, Keys, App Switches"]
    end

    subgraph Native Swift Application Core
        BUFFER["In-Memory Frame Ring Buffer\nLast 15-30s CVPixelBuffers"]
        FMP4["AVAssetWriter (Fragmented MP4)\nContinuous Video Stream to Disk"]
        
        VAD["Voice Activity Detection (VAD)\nSpeech Start / End Detection"]
        STT["Real-Time STT Engine\nSFSpeechRecognizer / Whisper CoreML"]
        
        TIMELINE["Temporal Interaction Indexer\nEvent Log (Time, Type, Coords, App)"]
        LOOKBACK["Dynamic Lookback Engine\nCorrelates Speech + Actions -> Target Window"]
    end

    subgraph Agent Bridge & Export
        DISPATCHER["Real-Time Event Dispatcher\nLocal WebSocket (ws://localhost:9292) / IPC"]
        BUNDLE["Artifact Package Builder\nvideo.mp4 + manifest.json + frames/"]
    end

    subgraph AI Coding Agent (e.g., Antigravity)
        AGENT["Active Agent Conversation\nIngests Trigger/Reaction Frames & Transcript"]
    end

    SCK --> BUFFER
    SCK --> FMP4
    MIC --> FMP4
    MIC --> VAD
    VAD --> STT
    EVENTS --> TIMELINE

    BUFFER --> LOOKBACK
    STT --> LOOKBACK
    TIMELINE --> LOOKBACK

    LOOKBACK --> DISPATCHER
    LOOKBACK --> BUNDLE
    DISPATCHER --> AGENT
```

---

## 2. Subsystem Breakdown

### 2.1 Video Capture Subsystem (`ScreenCaptureKit`)
* **Technology:** Apple's `ScreenCaptureKit` (`SCStream`, `SCStreamOutput`), introduced in macOS 12.3.
* **Characteristics:**
  * Captures directly from Apple Silicon Metal GPU surfaces (`IOSurface`).
  * Near-zero CPU overhead; battery friendly.
  * Native Retina resolution scaling or configurable 1080p / native mode.
* **Target Filtering:**
  * `SCContentFilter`: Configured for single-window capture (e.g. Simulator, Xcode, Chrome window) or whole displays.
  * Self-Exclusion: The recorder's own HUD and window are explicitly added to `filter.excludedWindows`.
* **Streaming Storage:**
  * Managed by `AVAssetWriter` configured for **Fragmented MP4 (`fMP4`)** output.
  * Avoids corrupt files if interrupted or terminated unexpectedly.
  * Allows reading or streaming active video chunks without closing the file.

### 2.2 In-Memory Video Frame Ring Buffer
* In parallel with writing to disk, `SCStreamOutput` delivers `CMSampleBuffer` frames.
* The application maintains a lightweight circular ring buffer of the last 15–30 seconds of `CVPixelBuffer` frames (or downsampled keyframe thumbnails at 2–5 FPS).
* **Instant Snapshot Extraction:**
  * When speech silence marks the end of a comment, snapshot frames can be grabbed **directly from memory in < 15 milliseconds** rather than waiting for `ffmpeg` or disk seeks.

### 2.3 Audio & Speech-to-Text Pipeline
* **Input:** `AVAudioEngine` tapping the default system input microphone.
* **Voice Activity Detection (VAD):**
  * CoreAudio RMS level meter combined with energy thresholding (or lightweight Silero VAD CoreML model).
  * Emits `speech_start` when volume/speech exceeds threshold for > 200ms.
  * Emits `speech_end` when silence persists for > 400ms.
* **Streaming Transcription:**
  * Primary: Apple `SFSpeechRecognizer` (`SFSpeechAudioBufferRecognitionRequest`).
    * Zero extra bundle size; on-device Apple offline speech models.
    * Emits partial tokens with accurate word-level timecodes.
  * Secondary (Optional): Local `whisper.cpp` / Whisper CoreML running on the Apple Neural Engine (ANE).

### 2.4 Interaction Logging (`CGEventTap`)
* Uses a passive macOS `CGEventTap` (`kCGEventTapOptionListenOnly`) to log user interactions during the recording:
  * Mouse Down / Click: Timestamp $t_{\text{action}}$, $(x, y)$ coordinates, click count.
  * Keyboard Shortcuts: Modifiers (`⌘`, `⌥`, `⇧`) and key codes.
  * Active App Changes: `NSWorkspace.didActivateApplicationNotification` captures application bundle ID and focused window title.
* Events are stored in a rolling timeline array alongside video timestamps.

---

## 3. The Dynamic Lookback Engine

The core design principle of director-mode feedback: **comments are latent**. Humans speak *after* seeing something happen.

```text
Time Line:
---|----------------------|-----------------------|-----------------------|--->
  t_action               t_speech_start          t_speech_mid            t_speech_end
  (Clicked Button)       ("Hey, this button...")  ("...did not submit...") (Silence)
  
  [<---------------------- DYNAMIC LOOKBACK WINDOW --------------------->]
```

### Calculation Rules

```swift
func calculateLookback(speechStart: TimeInterval, speechEnd: TimeInterval) -> LookbackWindow {
    let utteranceDuration = speechEnd - speechStart
    
    // 1. Identify preceding interactions within a reasonable threshold (e.g. 10s prior to speech)
    let candidateActions = interactionTimeline.filter { 
        $0.timestamp >= (speechStart - 10.0) && $0.timestamp <= speechStart 
    }
    
    let anchorAction = candidateActions.last // e.g. the most recent click or keystroke
    
    let windowStart: TimeInterval
    if let action = anchorAction {
        // Anchor 1.0s before the action that provoked the commentary
        windowStart = max(0, action.timestamp - 1.0)
    } else {
        // Fallback: Utterance length + proportional buffer
        let buffer = max(3.5, utteranceDuration * 0.25)
        windowStart = max(0, speechStart - buffer)
    }
    
    return LookbackWindow(
        startTime: windowStart,
        endTime: speechEnd,
        triggerTimestamp: anchorAction?.timestamp ?? (speechStart - 3.5),
        reactionTimestamp: speechEnd,
        anchorAction: anchorAction
    )
}
```

### Frame Extraction Strategy
* **Short comment (< 10 seconds):**
  * Extract **Trigger Frame** at $t_{\text{trigger}}$ (when the UI event happened).
  * Extract **Reaction Frame** at $t_{\text{reaction}}$ (when speech completed).
* **Extended comment (> 15 seconds):**
  * Extract a 4-to-6 frame carousel evenly distributed between $t_{\text{start}}$ and $t_{\text{end}}$.
  * Optionally slice a lightweight sub-clip (`.mp4`) of the specific lookback interval.

---

## 4. AI Agent Communication Pipeline

`xnk-capture` communicates with AI coding agents in two modes:

### Mode 1: Live WebSocket Event Stream (Real-Time)
* The app runs a lightweight local WebSocket server on `127.0.0.1:9292` (or Unix domain socket).
* An agent (or Antigravity plugin/sidecar) connects to this stream.
* On every finalized utterance, `xnk-capture` dispatches an event payload containing:
  * Full transcription text.
  * Precise timecode range.
  * Local file paths to extracted PNG frames (saved in a designated scratch folder).
  * App and interaction context (e.g. `window: "GameCanvas", clicked: (340, 520)`).

### Mode 2: Static Bundle Export (Post-Session)
* When recording stops, a structured bundle is assembled:
  ```text
  storage/recordings/capture_<id>_<timestamp>/
  ├── recording.mp4
  ├── manifest.json
  ├── frames/
  │   ├── frame_01_trigger_4.2s.png
  │   ├── frame_01_reaction_8.5s.png
  │   └── ...
  └── analysis.md  (Optional pre-compiled summary)
  ```
