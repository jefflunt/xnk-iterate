# XNK Iterate (`xnk-iterate`)

> **Voice-Directed Software Development & Multimodal AI Feedback Engine for macOS**

`xnk-iterate` is a standalone native macOS application designed to record screen interactions alongside synchronized live voice commentary. It automatically transcribes speech in real time, correlates commentary with dynamic lookback windows (anchored to preceding UI interactions and utterance durations), extracts dual-frame visual snapshots, and streams structured feedback events directly to AI coding agents.

---

## 1. The Vision & Origin

### Origin: The King's Claim In-Browser Holodeck
This project originated from an experiment in developing the RTS game *King's Claim* entirely through voice-directed gameplay recordings ([Jeff Lunt's original concept](https://x.com/jeffluntgames/status/2121602600261005548)). By capturing canvas video alongside live Speech-to-Text and telemetry in the browser, an AI agent could ingest director recordings, cross-reference trigger frames, and autonomously diagnose bugs and build features.

### The Leap: Universal macOS System-Wide Capture
While browser-based recording worked brilliantly for web games, software development spans native IDEs (Xcode, Cursor, VS Code), mobile simulators, terminals, design tools (Figma), and desktop apps. 

`xnk-iterate` extracts that exact director feedback loop into a **standalone, zero-overhead native macOS tool** that can monitor and critique **any application or workflow on your Mac**.

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
4. **Embedded Live Google Antigravity Session (Two-Way Voice + Vision Mode):**
   - Video continuously streams to disk using Fragmented MP4 (`fMP4`).
   - `xnk-iterate` **spawns and owns a live Antigravity session directly** via `agy` (`--input-format stream-json --output-format stream-json`, supporting `--continue` / `--conversation <id>` to resume any workspace thread, or `agy acp`).
   - **Auto-Stream + Batch Hotkey:** Every finalized utterance (~400–500ms silence) is injected immediately as a structured multimodal turn (transcript, lookback timecodes, interaction logs, and Trigger/Reaction PNG paths), with a hotkey to hold/batch multi-sentence thoughts before dispatch.
   - **Full Two-Way Conversational Voice:** Mirrors Antigravity's `/voice live` experience inside `xnk-iterate` — streaming spoken agent responses with hardware AEC barge-in (interrupt the agent just by speaking) and voice-driven permission approvals ("approve" / "deny") while you stay focused on your target app.
5. **Standalone Archive Bundles:**
   - Produces a self-contained `.xnk` (or `.director`) playtest folder containing the video, dual-frame snapshots (Trigger at $t_{\text{action}}$, Reaction at $t_{\text{speech}}$), and `manifest.json`.

---

## 3. Building & Running (v1 — Cross-Platform Go Desktop App)

`xnk-iterate` v1 is built in **Go** as a native cross-platform desktop UI application (`webview_go` using WKWebView on macOS, WebView2 on Windows, WebKitGTK on Linux), styled with the **XNK Design System v2.0** (`internal/ui/assets/xnk/`), with platform-specific build-tagged implementations (`_darwin.go`, `_windows.go`, `_linux.go`).

```bash
# Run full unit & integration test suite
make test

# Build binaries (bin/xnk-iterate and bin/xnk-mcp) and launch the desktop UI
make run

# Package a native macOS .app bundle with Screen Recording / Mic / Speech Info.plist keys
make mac-app
open "dist/XNK Iterate.app"
```

---

## 4. Project Documentation

* **[`ARCHITECTURE.md`](ARCHITECTURE.md):** Deep-dive system architecture, pipeline flowcharts, and component interaction.
* **[`SPECIFICATION.md`](SPECIFICATION.md):** Technical schemas, data models, video/audio formats, and WebSocket JSON-RPC event contracts.
* **[`UI_DESIGN.md`](UI_DESIGN.md):** SwiftUI UX specifications, window picker mechanics, and minimal floating HUD design.
* **[`ROADMAP.md`](ROADMAP.md):** Phased engineering implementation roadmap.
