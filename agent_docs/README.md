# Agent Documentation (`xnk-iterate` / `xnk-capture`)

This directory contains persistent context, architectural notes, and session logs for AI agents working on this repository.

## Project Overview

- **Repository:** `xnk-iterate` (`git@github.com:jefflunt/xnk-iterate.git`)
- **Product Name:** XNK Capture (`xnk-capture`)
- **Purpose:** Standalone native macOS application (Swift / SwiftUI, macOS 14+) for voice-directed software development and multimodal AI feedback. Captures GPU screen/window video (`ScreenCaptureKit`), real-time local speech transcription (`AVAudioEngine` + `SFSpeechRecognizer` / Whisper CoreML), and OS-level interaction events (`CGEventTap`), then correlates them via a Dynamic Lookback Engine and streams structured feedback events (`ws://127.0.0.1:9292`) and standalone `.xnk` bundles to AI coding agents.
- **License:** MIT (`LICENSE`)
- **Current Stage:** Design & specification complete; native Swift implementation has not started yet (no build or test suite yet).

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
4. **Dual Agent Delivery:** Live WebSocket stream (`ws://127.0.0.1:9292`) for real-time agent feedback plus post-session self-contained archive bundles (`recording.mp4`, `manifest.json`, `frames/`).

## Files in `agent_docs/`

- [`session-log.md`](session-log.md) — Chronological record of development sessions and decisions.
