# Session Log

## 2026-11-21 — MIT License & Agent Docs Initialization

### Summary
- Added root `LICENSE` (MIT License, Copyright 2026 Jeff Lunt).
- Initialized `agent_docs/` documentation system (`agent_docs/README.md` and `agent_docs/session-log.md`) capturing the initial architecture, technical specification, UI design, and engineering roadmap.
- Created `VERSION` set to `0.1.0` (tagged `v0.1.0`).

### Repository State
- Design documentation is in place (`README.md`, `ARCHITECTURE.md`, `SPECIFICATION.md`, `UI_DESIGN.md`, `ROADMAP.md`).
- No source code, build configuration, or test suite exists yet; next engineering milestone is Phase 1 of `ROADMAP.md` (initializing the macOS 14+ Xcode / Swift Package Manager project).

## 2026-11-21 — v1 Cross-Platform Go Desktop UI Implementation

### Summary
- Updated architecture, spec, UI design, and roadmap for an **owned Google Antigravity live session** (`agy` `stream-json` / ACP) with full two-way conversational voice (AEC barge-in + voice permission approvals) and auto-stream + batch-hold cadence.
- Built complete v1 in **Go** as a cross-platform native desktop GUI application (`cmd/xnk-iterate` + `cmd/xnk-mcp`) styled with the **XNK Design System v2.0** (`internal/ui/assets/xnk/` copied from `~/code/branding/systems/xnk`).
- Implemented modular cross-platform packages with full unit & integration test coverage:
  - `internal/model`, `internal/ringbuffer`, `internal/interaction` (`CGEventTap` on macOS + fallback), `internal/lookback`, `internal/batcher`
  - `internal/capture` (`CGWindowList` + `ffmpeg` fMP4 + ring buffer feeder on macOS; `gdigrab`/`x11grab` for Windows/Linux)
  - `internal/speech` (embedded Swift `AVAudioEngine` VoiceProcessingIO AEC + `SFSpeechRecognizer` helper on macOS; UI WebSpeech/manual fallback on other OSes)
  - `internal/voice` (interruptible TTS with sentence chunking and instant barge-in across macOS, Windows, Linux)
  - `internal/antigravity` (owned `agy` controller, NDJSON parser, multimodal prompt formatter, and Voice Permission Approval Router)
  - `internal/mcp` (Vision MCP JSON-RPC 2.0 server: `xnk_grab_frame`, `xnk_get_clip`, `xnk_session_status`)
  - `internal/bundle` (self-contained `manifest.json`, `recording.mp4`, `frames/`, `analysis.md` exporter)
  - `internal/session`, `internal/server` (HTTP + Gorilla WebSocket hub), `internal/ui` (`webview_go` native window + macOS `NSWindowSharingNone` self-exclusion + Floating HUD mode).
- Verified `go test -v ./...`, `make mac-app`, `swiftc` compilation of `helper_darwin.swift`, and `GOOS=linux` + `GOOS=windows` cross-compilation.
- Renamed all branding and documentation references from `XNK Capture` / `xnk-capture` to `XNK Iterate` / `xnk-iterate` (`dist/XNK Iterate.app`, `com.xnk.iterate`).
- Hardened `.gitignore` to exclude binaries, build artifacts, logs, caches, and runtime recordings while preserving `cmd/` and `internal/`.
- Bumped `VERSION` to `1.0.0` (`v1.0.0`).

## 2026-11-21 — v1.1.0 Semi-Detachable Multi-Monitor Capture Box & Trimmed Studio UI

### Summary
- Redesigned the left capture pane into a pure interactive **Selection Drag Box** canvas with 8 resize handles, live `(W × H, x, y)` readout, display switcher, and a single floating bottom dock (`Start / Stop`, `Pause`, live mic waveform, `Batch ⌥B`, `Discard`).
- Implemented `CaptureBoxController` (`internal/ui/capturebox.go`, `capturebox_darwin.go`, `capturebox_darwin.m`, `capturebox_other.go`):
  - Detachable transparent borderless native macOS `NSWindow` (`NSWindowSharingNone`) moveable and resizable across multiple monitors independently of the Antigravity sidebar.
  - Automatically locks position & size (`region.Locked = true`) and switches to full click-through (`ignoresMouseEvents = YES`) with a dashed Crimson `● REC LOCKED` frame while recording; unlocks on stop.
- Updated `internal/capture/capturer_darwin.go` to crop `ffmpeg` (`-vf crop=...`) and `screencapture -R x,y,w,h` to the exact screen-space `CaptureRegion`.
- Relocated Workspace path, Conversation ID, `--continue`, and OS permission readiness badges to the `⚙ Settings` drawer and compact sidebar workspace pill.
- Added `TestCaptureRegionDetachAndLockBehavior` and bumped `VERSION` to `1.1.0` (`v1.1.0`).

## 2026-11-21 — v1.2.0 App Icon, AGY Wire Fix, Always-Detached Capture Box, Wide Chat UI, Folder Picker & Diagnostic Logs

### Summary
- Generated and bundled native macOS Retina app icon (`assets/AppIcon.icns`, `CFBundleIconFile = AppIcon`).
- Fixed `agy --input-format stream-json --output-format stream-json` wire protocol in `internal/antigravity/controller.go` (`{"event":"user","message":{"content":"..."}}` input; nested `step_update.text_delta` and `result.response`/`result.error` parsing) and added live streaming AI reply bubble in chat.
- Set AI Text-to-Speech (TTS) to **OFF by default** with a prominent top-bar `AI Voice: OFF / ON` toggle.
- Made the transparent native macOS Capture Box **always detached** (removed redock mode) and redesigned the main Control Plane into a wide landscape standard chat interface (`1120×740`).
- Fixed Stop button reliability via immediate non-blocking state reset/unlock in `Orchestrator.StopSession()` plus a `POST /api/stop` REST fallback.
- Added structured rotating diagnostic logging (`internal/applog`, `~/Library/Logs/XNK Iterate/xnk-iterate.log`) with live log tail viewer inside `⚙ Settings` (`GET /api/logs`).
- Added native macOS folder picker (`NSOpenPanel`, `POST /api/pick-folder`, `📂 Browse…` button) for selecting the Antigravity workspace directory.
- Bumped `VERSION` to `1.2.0` (`v1.2.0`).
