# Agent Documentation (`xnk-iterate`)

This directory contains persistent context, architectural notes, and session logs for AI agents working on this repository.

## Project Overview

- **Repository:** `xnk-iterate` (`git@github.com:jefflunt/xnk-iterate.git`)
- **Product Name:** XNK Iterate (`xnk-iterate`)
- **Purpose:** Standalone native macOS application (Swift / SwiftUI, macOS 14+) for voice-directed software development and multimodal AI feedback. Captures GPU screen/window video (`ScreenCaptureKit`), real-time local speech transcription (`AVAudioEngine` + `SFSpeechRecognizer` / Whisper CoreML), and OS-level interaction events (`CGEventTap`), then correlates them via a Dynamic Lookback Engine and streams structured feedback events (`ws://127.0.0.1:9292`) and standalone `.xnk` bundles to AI coding agents.
- **License:** MIT (`LICENSE`)
- **Current Stage:** v2.1.0 implemented in Go as a native macOS-first **Google Antigravity Partner Companion** (`webview_go` floating HUD + embedded XNK Design System v2.0 CSS + always-detached multi-monitor Capture Box + Carbon global hotkeys + `agentapi` bridge + conversation-scoped brain frame storage). Build with `make build` / `make mac-app`, test with `make test`.

## Core Project Documents

- [`README.md`](../README.md) — Vision, core capabilities, and build/run instructions.
- [`session-log.md`](session-log.md) — Chronological record of development sessions, releases, and architectural changes.

## Key Architectural Decisions

1. **Zero-CPU GPU Capture:** Use `ScreenCaptureKit` (`SCStream`) writing Fragmented MP4 (`fMP4`) via `AVAssetWriter` plus an in-memory 15–30s `CVPixelBuffer` ring buffer for <15ms snapshot extraction.
2. **Dynamic Lookback:** Anchor utterance lookback windows to preceding `CGEventTap` interactions (1.0s before the anchor action within 10s prior to speech), falling back to proportional utterance duration buffering.
3. **Self-Excluding HUD:** Floating SwiftUI HUD (`NSWindow.level = .floating`) explicitly excluded via `SCContentFilter.excludedWindows`.
4. **Google Antigravity Partner Companion (`agentapi` + `/xnk-iterate` Skill):** Instead of spawning its own `agy` chat UI, `xnk-iterate` binds to any existing Google Antigravity Desktop conversation via `POST http://127.0.0.1:9292/api/bind` (receiving `conversation_id`, `ls_address`, `csrf_token`, and `agentapi_exe` from the `/xnk-iterate` skill) and injects voice turns + lookback timestamps + Trigger/Reaction frames via `agentapi send-message`.
5. **Voice-Activated Start + Customizable Global Carbon Hotkeys:** Auto-send and batch modes are intentionally removed. Speech accumulates continuously across pauses (VAD marks `speechStart` for lookback) until the user triggers Send (default `⌥↩`) or Discard (default `⌥⌫`) via macOS Carbon `RegisterEventHotKey` (customizable live in Settings without Accessibility permissions).

## Repository Conventions & Preferences

- **Skip HQ Publication Queue:** Do **not** read or append to `~/code/hq/data/PUBLIC_QUEUE.md` (Phase 6 of `/wrapup`) for this repository until the user explicitly states otherwise.

## Files in `agent_docs/`

- [`session-log.md`](session-log.md) — Chronological record of development sessions and decisions.
