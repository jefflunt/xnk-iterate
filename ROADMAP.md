# Engineering Roadmap (`xnk-iterate`)

This document defines the implementation phases for building `xnk-iterate` from scratch as a native macOS application.

---

## Phase 1: Native Video & Audio Capture Engine
*Goal: Capture window or display video at 60 FPS and microphone audio into Fragmented MP4 with zero CPU lag.*

- [ ] Initialize macOS Xcode / Swift Package Manager project target (`macOS 14+` deployment target).
- [ ] Implement `ScreenCaptureManager` wrapping `ScreenCaptureKit` (`SCStream`, `SCStreamOutput`).
- [ ] Add display enumeration and window discovery via `SCShareableContent`.
- [ ] Configure `AVAssetWriter` for Fragmented MP4 (`fMP4`) video chunks (`H.264` hardware encoder).
- [ ] Implement `AudioCaptureManager` via `AVAudioEngine` to capture 48kHz mono mic input.
- [ ] Multiplex audio and video streams into a synchronized MP4 file.
- [ ] Implement an in-memory `CVPixelBuffer` rolling ring buffer (last 30 seconds).

---

## Phase 2: Streaming Speech Recognition & VAD
*Goal: Detect when the user speaks, transcribe live, and emit utterance boundary events.*

- [ ] Implement `VoiceActivityDetector` using audio energy levels / RMS metering to flag speech start and silence boundaries.
- [ ] Integrate Apple `SFSpeechRecognizer` with continuous `SFSpeechAudioBufferRecognitionRequest`.
- [ ] Implement real-time token streaming with word-level timestamps.
- [ ] Add silence timeout debounce (400–500ms) to trigger utterance finalization.
- [ ] Benchmark speech transcription latency (< 300ms from end-of-speech).

---

## Phase 3: Interaction Tracking & Dynamic Lookback Engine
*Goal: Correlate what the user is saying with what just happened on screen.*

- [ ] Implement `InteractionTracker` using `CGEventTap` to record mouse clicks, coordinates, and keystrokes.
- [ ] Add `NSWorkspace` notifications to track active application bundle ID and window title switches.
- [ ] Implement `DynamicLookbackCalculator`:
  - Calculate utterance duration ($t_{\text{end}} - t_{\text{start}}$).
  - Search preceding interaction events within $[t_{\text{start}} - 10\text{s}, t_{\text{start}}]$ to locate the anchor action.
  - Dynamically scale lookback window from short reactions ($5\text{s}$) to extended critiques ($2\text{m}+$).
- [ ] Implement instant snapshot extraction from the in-memory ring buffer (Trigger Frame at $t_{\text{trigger}}$, Reaction Frame at $t_{\text{reaction}}$).

---

## Phase 4: Agent Bridge & Artifact Bundler
*Goal: Stream live events to AI coding agents and export structured playtest packages.*

- [ ] Implement local WebSocket server (`ws://127.0.0.1:9292`) using Swift `NWListener`.
- [ ] Define JSON-RPC / event payload schema (`capture_started`, `director_utterance`, `capture_completed`).
- [ ] Implement `BundleExporter`:
  - Output structured folder: `recording.mp4`, `manifest.json`, and `frames/`.
  - Save extracted PNG snapshots with timestamp filenames.
- [ ] Build a companion CLI command (`xnk-iterate analyze <path>`) for headless script automation.

---

## Phase 5: Native SwiftUI HUD & Window Picker Experience
*Goal: Provide a polished, built-in macOS experience resembling Cmd+Shift+5.*

- [ ] Implement interactive window selection overlay:
  - Transparent full-screen window with mouse hover tracking over `SCWindow` rects.
  - Cyan rounded bounding box snapping to target windows.
- [ ] Build `FloatingDirectorHUD` in SwiftUI:
  - Recording duration timer with pulsing REC indicator.
  - Real-time audio waveform visualizer.
  - Live transcription text ribbon.
  - Pause, Stop, and Discard buttons.
- [ ] Configure window level (`.floating`) and add HUD window ID to `SCContentFilter.excludedWindows`.
- [ ] Register global system hotkey (`⌥ + Shift + C` to toggle capture, `Space` to pause).

---

## Phase 6: Embedded Google Antigravity Live Voice + Vision Session
*Goal: Spawn and own a live Google Antigravity (`agy`) session directly inside `xnk-iterate`, delivering a full two-way conversational voice + screen vision loop modeled after `/voice live`.*

- [ ] Implement `AntigravitySessionController` in Swift:
  - Spawn and manage an owned `agy` process (`--input-format stream-json --output-format stream-json` / `agy acp`) scoped to the selected project workspace, with support for `--continue` / `--conversation <id>`.
  - Parse streamed NDJSON/ACP events (assistant text deltas, tool calls, permission requests, turn completion) in real time.
- [ ] Implement **Auto-Stream + Batch-Hold Dispatcher**:
  - Auto-dispatch finalized utterances (~400–500ms silence) with Trigger/Reaction PNG paths and `CGEventTap` metadata straight to `agy`.
  - Add Batch-Hold hotkey (`⌥ + B`) to accumulate multi-sentence thoughts into a single combined multimodal turn.
- [ ] Implement **Full Two-Way Conversational Voice**:
  - Enable `AVAudioEngine` Voice Processing I/O (hardware AEC) so agent speech never re-triggers mic STT.
  - Stream spoken agent replies via `AVSpeechSynthesizer` (sentence-chunked) with instant VAD barge-in interruption (`> 200ms` user speech stops TTS immediately).
  - Build **Voice Permission Approval Router**: surface pending `agy` permission prompts on the HUD, speak a concise confirmation question, and accept `"approve"` / `"deny"` voice commands (or `⌘Y` / `⌘N`).
- [ ] Wire embedded local MCP server (`xnk_grab_frame`, `xnk_get_clip`, `xnk_session_status`) into the spawned `agy` session for on-demand visual inspection.
- [ ] Test end-to-end live loop: launch capture over target app -> speak a bug report -> `agy` inspects Trigger/Reaction frames, asks voice approval to run tests, speaks status update while user interrupts mid-sentence to steer the next fix.
