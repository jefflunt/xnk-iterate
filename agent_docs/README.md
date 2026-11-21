# Agent Documentation (`xnk-iterate`)

This directory contains persistent context, architectural notes, and session logs for AI agents working on this repository.

## Project Overview

- **Repository:** `xnk-iterate` (`git@github.com:jefflunt/xnk-iterate.git`)
- **Product Name:** XNK Iterate (`xnk-iterate`)
- **Purpose:** Standalone native macOS application (Swift / SwiftUI, macOS 14+) for voice-directed software development and multimodal AI feedback. Captures GPU screen/window video (`ScreenCaptureKit`), real-time local speech transcription (`AVAudioEngine` + `SFSpeechRecognizer` / Whisper CoreML), and OS-level interaction events (`CGEventTap`), then correlates them via a Dynamic Lookback Engine and streams structured feedback events (`ws://127.0.0.1:9292`) and standalone `.xnk` bundles to AI coding agents.
- **License:** MIT (`LICENSE`)
- **Current Stage:** v1 implemented in Go as a cross-platform native desktop GUI application (`webview_go` + embedded XNK Design System v2.0 CSS + macOS native implementations with Windows/Linux build-tagged parity). Build with `make build` / `make mac-app`, test with `make test`.

## Core Project Documents

- [`README.md`](../README.md) — Vision, origin (*King's Claim* in-browser holodeck), and core capabilities.
- [`ARCHITECTURE.md`](../ARCHITECTURE.md) — System architecture, subsystem breakdown, Dynamic Lookback Engine rules, and agent communication modes.
- [`SPECIFICATION.md`](../SPECIFICATION.md) — Video/audio encoding specs, Swift data models (`CaptureManifest`, `Utterance`, `InteractionEvent`, `FrameSnapshot`), WebSocket JSON event schemas, and required macOS permissions.
- [`UI_DESIGN.md`](../UI_DESIGN.md) — Window picker UX (`Cmd+Shift+5`-style), floating director HUD, self-exclusion from `SCContentFilter`, menu bar tray, and SwiftUI scaffold.
- [`ROADMAP.md`](../ROADMAP.md) — 6-phase engineering roadmap from capture engine through Antigravity skill integration.

## Key Architectural Decisions

1. **Zero-CPU GPU Capture:** Use `ScreenCaptureKit` (`SCStream`) writing Fragmented MP4 (`fMP4`) via `AVAssetWriter` plus an in-memory 15–30s `CVPixelBuffer` ring buffer for <15ms snapshot extraction.
2. **Dynamic Lookback:** Anchor utterance lookback windows to preceding `CGEventTap` interactions (1.0s before the anchor action within 10s prior to speech), falling back to proportional utterance duration buffering.
3. **Self-Excluding HUD:** Floating SwiftUI HUD (`NSWindow.level = .floating`) explicitly excluded via `SCContentFilter.excludedWindows`.
4. **Embedded Google Antigravity Live Session:** `xnk-iterate` directly spawns and owns an `agy` session (`--input-format stream-json --output-format stream-json` / `agy acp`, supporting `--continue` / `--conversation <id>`) with an embedded local MCP server (`xnk_grab_frame`, `xnk_get_clip`), auto-stream + batch-hold utterance dispatch, and full two-way conversational voice (AEC barge-in interruption via `AVAudioEngine` Voice Processing I/O + voice-driven permission approvals).

## Repository Conventions & Preferences

- **Skip HQ Publication Queue:** Do **not** read or append to `~/code/hq/data/PUBLIC_QUEUE.md` (Phase 6 of `/wrapup`) for this repository until the user explicitly states otherwise.

## Files in `agent_docs/`

- [`session-log.md`](session-log.md) — Chronological record of development sessions and decisions.
