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

## 2026-11-21 — v1.2.1 Transparent macOS Squircle App Icon & Dock Icon Binding

### Summary
- Applied a proper transparent RGBA macOS squircle mask + subtle drop shadow and metallic hairline border to `assets/icon_1024.png` (`hasAlpha: yes`) and rebuilt `assets/AppIcon.icns` with all 11 Apple ICNS sizes (`16px`–`1024px`).
- Embedded `internal/ui/assets/icon.png` to set `[NSApp setApplicationIconImage:]` at runtime in `internal/ui/window_darwin.go` (so Dock & Cmd+Tab always show the icon even when run via `make run`) and added `<link rel="icon">` in `index.html`.
- Added `touch "$(MAC_APP)"` in `Makefile` to trigger immediate macOS Finder icon cache refresh, and bumped `VERSION` to `1.2.1` (`v1.2.1`).

## 2026-11-21 — v1.2.2 Native macOS Main Menu (`Cmd+Q` Quit & Standard Edit/Window Shortcuts)

### Summary
- Added native Cocoa `NSMenu` (`setupMacMainMenu`) in `internal/ui/window_darwin.go` with App menu (`Cmd+Q` Quit, `Cmd+H` Hide), Edit menu (`Cmd+Z/X/C/V/A`), and Window menu (`Cmd+W` Close, `Cmd+M` Minimize) so `Cmd+Q` quits cleanly without force-quitting.
- Bumped `VERSION` to `1.2.2` (`v1.2.2`).

## 2026-11-21 — v1.3.0 `agy acp` Interactive Approvals, 1.8s Pause, Collapsible Tool Steps, Stop Fix & Clean App Termination

### Summary
- Fixed `StopSession()` `RWMutex` self-deadlock (`o.elapsedLocked()`) + vivid solid Crimson Stop button + `TestStopSessionDoesNotDeadlock`.
- Upgraded Antigravity integration (`internal/antigravity/controller.go`) from headless `stream-json` to **`agy acp --stdio` (Agent Client Protocol JSON-RPC 2.0)** with native ACP base64 image attachments and true interactive `session/request_permission` prompts (`Allow once ⌘Y`, `Allow always`, `Reject ⌘N`, plus voice approvals).
- Raised default auto-send silence pause from `0.48s` to **`1.8s`** (configurable `1.2s / 1.8s / 2.5s` in Settings, plus Browser STT debounce) while subtracting `silence_offset` so `speechEnd` and lookback timestamps stay exact.
- Collapsed noisy tool call chips into a single live status pill while active and a collapsible `▸ N tool steps` `<details>` disclosure inside each AI reply.
- Added persistent app config (`internal/config/store.go`, `~/Library/Application Support/XNK Iterate/config.json`) remembering workspace path, silence pause, AI Voice mute state, and Capture Box geometry across restarts.
- Added `NSWindowWillCloseNotification` + `NSApplicationWillTerminateNotification` cleanup observers (`internal/ui/window_darwin.m`) so closing the main window or pressing `Cmd+Q` always destroys the detached Capture Box, stops child helpers, and exits cleanly with zero background lingering.
- Bumped `VERSION` to `1.3.0` (`v1.3.0`).

## 2026-11-21 — v1.3.1 Root Documentation Cleanup

### Summary
- Removed redundant root ALL CAPS design docs (`ARCHITECTURE.md`, `ROADMAP.md`, `SPECIFICATION.md`, `UI_DESIGN.md`) now superseded by `agent_docs/`.
- Updated `README.md` and `agent_docs/README.md` links accordingly and bumped `VERSION` to `1.3.1` (`v1.3.1`).

## 2026-11-21 — v2.0.0 Antigravity Partner Companion (`agentapi`), Customizable Global Hotkeys & Speech/Capture Fixes

### Summary
- **Antigravity Partner Companion Pivot (`internal/antigravity/controller.go`, `internal/server/server.go`, `skills/xnk-iterate/SKILL.md`):**
  - Replaced the embedded `agy` chat wrapper with a direct **`agentapi` partner bridge** (`get-conversation-metadata`, `send-message --title="XNK Iterate Director"`) that injects voice commentary, lookback timecodes, OS interactions, and Trigger/Reaction frame paths directly into any existing Google Antigravity Desktop conversation.
  - Added `/xnk-iterate` Antigravity skill (`skills/xnk-iterate/SKILL.md`, installed to `~/.gemini/config/skills/xnk-iterate/SKILL.md`) binding via `POST http://127.0.0.1:9292/api/bind` with `conversation_id`, `ls_address`, `csrf_token`, and `agentapi_exe` (persisted in `config.json` and injected into `agentapi`'s environment).
  - Redesigned the desktop UI into a sleek `960×108` floating Companion HUD (expandable to `960×500` drawer for Captured Turns, Settings, and Diagnostic Logs) alongside the always-detached multi-monitor Capture Box.
- **Voice-Activated Start + Customizable Global macOS Hotkeys (`internal/ui/hotkey_darwin.{m,go}`, `internal/model/hotkeys.go`):**
  - Removed auto-send silence timers and batch send mode entirely.
  - Implemented system-wide macOS Carbon hotkeys (`RegisterEventHotKey`, no Accessibility permission needed) defaulting to **`⌥↩`** (Send Turn) and **`⌥⌫`** (Discard Turn), plus an interactive click-to-record shortcut customizer in **⚙ Settings** with live re-registration and persistence.
- **Speech & macOS Screen Recording Fixes (`internal/speech/helper_darwin.swift`, `internal/capture/capturer_darwin.go`):**
  - Fixed native macOS speech recognition by removing `setVoiceProcessingEnabled(true)`, using native hardware mic format (`format: nil`), removing forced offline-only recognition, adding `taskGeneration` guards against stale callbacks on flush/discard, and adding full speech diagnostics + 250ms Send fallback.
  - Guarded `darwinCapturer.Start` with `CGPreflightScreenCaptureAccess()` to prevent repeated macOS Screen Recording prompts after unsigned rebuilds, and added ad-hoc `codesign --force --deep --sign -` to `Makefile`.
- Bumped `VERSION` to `2.0.0` (`v2.0.0`).

## 2026-11-21 — v2.1.0 Conversation-Scoped Brain Frame Storage (Zero Permission Prompts)

### Summary
- Updated `Orchestrator.FramesOutputDir()` (`internal/session/orchestrator.go`) to dynamically write Trigger, Reaction, Carousel, and MCP on-demand frames directly into the currently linked Antigravity conversation's brain directory (`<appDataDir>/brain/<conversation_id>/xnk-frames/<capture_id>/`, defaulting to `~/.gemini/antigravity/brain/...`), eliminating repeated Antigravity external-file read prompts.
- Unlinking or re-linking a different conversation automatically switches the target brain directory for all subsequent frames (falling back to the local archive bundle `frames/` directory when unlinked).
- Updated `/api/bind` (`internal/server/server.go`), `config.AppConfig` (`internal/config/store.go`), and `skills/xnk-iterate/SKILL.md` to capture and persist `ANTIGRAVITY_APP_DATA_DIR`, and updated `/api/frame` to serve HUD thumbnails/lightbox images from both `StorageRoot()` and `BrainRoot()`.
- Added `TestFramesOutputDirFollowsLinkedConversation` and bumped `VERSION` to `2.1.0` (`v2.1.0`).

## 2026-11-21 — v2.1.1 Compact Human-First Director Message Format

### Summary
- Simplified `FormatDirectorTurn` (`internal/antigravity/controller.go`) to put the user's spoken words first, followed by a single compact metadata & clickable frame link footer (`` `#1 · 0.4s–6.4s` · [trigger](...) · [reaction](...) ``), eliminating the redundant `[XNK ITERATE — LIVE DIRECTOR FEEDBACK]` banner and duplicated display resolution header.
- Updated `skills/xnk-iterate/SKILL.md` (and `~/.gemini/config/skills/xnk-iterate/SKILL.md`), `TestFormatDirectorTurn`, and bumped `VERSION` to `2.1.1` (`v2.1.1`).

## 2026-11-21 — v2.2.0 Borderless Draggable HUD, Compact 3-Bar Meter & Auto-Scrolling Live Transcript

### Summary
- Converted the floating Companion HUD into a sleek **borderless macOS window** (`960×68` collapsed, `960×470` expanded) by hiding the native titlebar and traffic lights (`NSWindowStyleMaskFullSizeContentView` + hidden standard buttons) while retaining full keyboard focus and native rounded corners/shadow (`internal/ui/window_darwin.m`, `internal/ui/desktop.go`).
- Bound Apple's native `[win performWindowDragWithEvent:]` via `window.__xnkStartDrag()` so the HUD drags smoothly across monitors from the **XNK** badge or any non-button area of the bar (`⌘Q` quits cleanly).
- Shrunk the audio level meter from `76px` to a compact `14px` 3-bar indicator and made the live transcript ribbon auto-scroll horizontally to the newest spoken words with a subtle left-edge fade mask (`internal/ui/assets/{index.html,styles.css,app.js}`).
- Bumped `VERSION` to `2.2.0` (`v2.2.0`).

## 2026-11-21 — v2.2.1 Borderless HUD Initial Height Fix

### Summary
- Applied `configureMacWindow` (`NSWindowStyleMaskFullSizeContentView`) synchronously before `w.SetSize` and increased compact HUD height from `68px` to `88px` (`960×88` collapsed, `960×490` expanded) so the borderless HUD launches at full unclipped height.
- Bumped `VERSION` to `2.2.1` (`v2.2.1`).

## 2026-11-22 — v2.3.0 Persistent HUD Width & Locked Vertical Resizing

### Summary
- Locked vertical resizing on the floating macOS Companion HUD (`internal/ui/window_darwin.m`, `internal/ui/desktop.go`) by fixing `minSize.height == maxSize.height` (`72px` collapsed, `480px` expanded) while preserving smooth horizontal resizing (`760px`–`4096px`) and anchoring the top edge when toggling the drawer.
- Added persistent control plane width (`WindowWidth` in `internal/config/store.go`, wired through `internal/session/orchestrator.go`, `internal/ui/desktop.go`, and `cmd/xnk-iterate/main.go`) observed via `NSWindowDidResizeNotification` and `NSWindowDidEndLiveResizeNotification` so the HUD reopens at the exact width the user left it and preserves custom width across drawer toggles.
- Added unit tests (`internal/config/store_test.go`, `internal/ui/desktop_test.go`) covering width normalization, persistence, and live-resize deduplication.
- Bumped `VERSION` to `2.3.0` (`v2.3.0`).
