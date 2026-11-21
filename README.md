# XNK Capture (`xnk-capture`)

> **Voice-Directed Software Development & Multimodal AI Feedback Engine for macOS**

`xnk-capture` is a standalone native macOS application designed to record screen interactions alongside synchronized live voice commentary. It automatically transcribes speech in real time, correlates commentary with dynamic lookback windows (anchored to preceding UI interactions and utterance durations), extracts dual-frame visual snapshots, and streams structured feedback events directly to AI coding agents.

---

## 1. The Vision & Origin

### Origin: The King's Claim In-Browser Holodeck
This project originated from an experiment in developing the RTS game *King's Claim* entirely through voice-directed gameplay recordings ([Jeff Lunt's original concept](https://x.com/jeffluntgames/status/2121602600261005548)). By capturing canvas video alongside live Speech-to-Text and telemetry in the browser, an AI agent could ingest director recordings, cross-reference trigger frames, and autonomously diagnose bugs and build features.

### The Leap: Universal macOS System-Wide Capture
While browser-based recording worked brilliantly for web games, software development spans native IDEs (Xcode, Cursor, VS Code), mobile simulators, terminals, design tools (Figma), and desktop apps. 

`xnk-capture` extracts that exact director feedback loop into a **standalone, zero-overhead native macOS tool** that can monitor and critique **any application or workflow on your Mac**.

---

## 2. Core Capabilities

1. **Native Window & Screen Capture:**
   - Interactive window picker modeled after macOS `Cmd + Shift + 5`.
   - Hardware-accelerated GPU capture via `ScreenCaptureKit` (`SCStream`) with zero CPU overhead.
   - Multi-target support: select an entire display, a specific window, or an application bundle (with self-exclusion of the capture HUD).
2. **Local, Real-Time Voice Transcription:**
   - Continuous audio capture via `AVAudioEngine` / CoreAudio with Voice Activity Detection (VAD).
   - Real-time offline transcription using Apple's built-in `SFSpeechRecognizer` or on-device Whisper CoreML running on the Apple Neural Engine (ANE).
3. **Dynamic Lookback Window (Beyond 5 Seconds):**
   - Utterances are automatically mapped to when an in-game or in-app event occurred, not just when the speech ended.
   - **Dynamic Duration Scaling:** If you make a quick comment, lookback spans $[t_{\text{start}} - 3\text{s}, t_{\text{end}}]$. If you speak for a 2-minute monologue, the lookback dynamically scales to cover the entire preceding interaction ($2.5+$ minutes).
   - **Interaction Anchoring (`CGEventTap`):** Correlates speech start with recent mouse clicks, key presses, and window focus changes to anchor the lookback to the root action.
4. **Real-Time Streaming to AI Agents:**
   - Video continuously streams to disk using Fragmented MP4 (`fMP4`).
   - The instant an utterance ends, a structured JSON event (with transcript, timecodes, interaction logs, and extracted PNG snapshots) is emitted over a local WebSocket / IPC pipe to an active agent (e.g. Antigravity).
   - Agents can create tasks, edit code, and trigger rebuilds in real time while you continue testing.
5. **Standalone Archive Bundles:**
   - Produces a self-contained `.xnk` (or `.director`) playtest folder containing the video, dual-frame snapshots (Trigger at $t_{\text{action}}$, Reaction at $t_{\text{speech}}$), and `manifest.json`.

---

## 3. Project Documentation

* **[`ARCHITECTURE.md`](ARCHITECTURE.md):** Deep-dive system architecture, pipeline flowcharts, and component interaction.
* **[`SPECIFICATION.md`](SPECIFICATION.md):** Technical schemas, data models, video/audio formats, and WebSocket JSON-RPC event contracts.
* **[`UI_DESIGN.md`](UI_DESIGN.md):** SwiftUI UX specifications, window picker mechanics, and minimal floating HUD design.
* **[`ROADMAP.md`](ROADMAP.md):** Phased engineering implementation roadmap.
