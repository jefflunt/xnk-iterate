# UI & UX Design (`xnk-iterate`)

This document describes the user experience, interaction design, and SwiftUI architecture for `xnk-iterate`.

---

## 1. Design Philosophy

`xnk-iterate` is designed to feel like a native, built-in part of macOS—combining the clean familiarity of macOS’s `Cmd + Shift + 5` screen capture overlay with a floating, zero-friction developer HUD.

* **Never get in the way:** The UI must be lightweight, self-excluding from the video stream, and completely keyboard-drivable.
* **Instant visual feedback:** Real-time speech transcription visualizer so the director knows immediately that their speech is recognized clearly.
* **Effortless targeting:** One-click window snapping or drag-to-select regions.

---

## 2. Window Picker Experience (Pre-Recording)

Modeled directly after macOS `Cmd + Shift + 5`:

```text
┌─────────────────────────────────────────────────────────────────┐
│                                                                 │
│   ┌─────────────────────────────────────────────────────────┐   │
│   │  [Xcode / Simulator Window]                             │   │
│   │                                                         │   │
│   │  ✦ Hover highlights window with rounded cyan border     │   │
│   │  ✦ Click snaps target to this window                    │   │
│   └─────────────────────────────────────────────────────────┘   │
│                                                                 │
│          ┌───────────────────────────────────────────┐          │
│          │ [ 🖥 Full Screen ] [ 🪟 Window ] [ ⚲ Area ] │          │
│          │  Mic: MacBook Pro Microphone ▾           │          │
│          │  [ ⏺ Start Director Session ]             │          │
│          └───────────────────────────────────────────┘          │
└─────────────────────────────────────────────────────────────────┘
```

1. **Global Trigger:** Press `⌥ + Shift + C` (Option-Shift-Capture) from anywhere in macOS.
2. **Interactive Window Snapping:**
   - As you move the mouse across the desktop, windows under the cursor are highlighted with a smooth, semi-transparent frosted-glass overlay and blue/cyan border.
   - Clicking locks the capture target to that window's `CGWindowID`.
   - If the window moves, resizes, or minimizes, `ScreenCaptureKit` continues tracking that specific window automatically.

---

## 3. The Floating HUD (During Recording)

While recording, a compact, frosted-glass floating bar sits unobtrusively on screen (by default at the bottom-center or anchored to the menu bar):

```text
┌────────────────────────────────────────────────────────────────────────────────────────────┐
│ ● REC 02:15 │ 🎤 [ılı|ı|ı] │ "The scoreboard colors..." │ 🔊 AGY: Speaking / Working │ [⏸][⏹]
└────────────────────────────────────────────────────────────────────────────────────────────┘
  └─ [Pending Approval]: Run `swift test`?  Say "Approve" / "Deny"  [✓ ⌘Y] [✕ ⌘N]
```

### Components
1. **Recording Indicator:** Glowing red pulse with elapsed session timecode.
2. **Live Audio Meter:** Mini waveform / volume meter showing real-time mic activity (with AEC active during agent speech).
3. **Live Transcript Ribbon:** Compact marquee/ticker displaying live recognized speech tokens, plus a `BATCH` badge when holding/accumulating a multi-sentence thought.
4. **Embedded Antigravity Voice & Status Pill:** Shows the owned `agy` session state (`Listening`, `Thinking`, `Working: <tool>`, `Speaking` with animated equalizer, or `Awaiting Approval`). Speaking while `AGY` is talking triggers instant barge-in interruption.
5. **Voice Permission Approval Strip:** Expands smoothly beneath the HUD pill whenever `agy` requests tool permission, speaking a brief prompt and accepting voice commands (`"Approve"` / `"Deny"`) or keyboard shortcuts (`⌘Y` / `⌘N`).
6. **Quick Controls:**
   - `⏸ Pause / Resume` (Hotkey: `Space` or `⌥ + Space`)
   - `📦 Hold / Batch Thought` (Hotkey: `⌥ + B` — pauses auto-stream to accumulate multiple sentences, then dispatches as one turn)
   - `⏹ Stop & Export` (Hotkey: `Escape` or `⌘ + Enter`)
   - `🗑 Discard Last Utterance` (Hotkey: `⌘ + Delete`) — Purges a misspoken comment before dispatch.

### Self-Exclusion Guarantee
* The HUD window uses `NSWindow.level = .floating`.
* When configuring the `SCContentFilter`, the HUD's `windowNumber` is explicitly appended to `filter.excludedWindows`, ensuring the floating controls **never appear in the recorded video stream**.

---

## 4. Menu Bar Tray & Status

For minimalists who prefer no floating window on screen:
* Menu bar icon (`waveform.badge.magnifyingglass`).
* Menu items:
  * `Start Director Recording (⌥⇧C)`
  * `Capture Window...`
  * `Capture Entire Screen`
  * `Select Microphone ▾`
  * `Agent Integration: Connected (ws://localhost:9292) ●`
  * `Open Recent Recordings...`

---

## 5. SwiftUI Scaffold Concept

```swift
import SwiftUI

struct FloatingDirectorHUD: View {
    @ObservedObject var captureManager: CaptureManager
    @ObservedObject var speechManager: SpeechManager

    var body: some View {
        HStack(spacing: 12) {
            // REC Status
            HStack(spacing: 6) {
                Circle()
                    .fill(Color.red)
                    .frame(width: 8, height: 8)
                Text(captureManager.formattedDuration)
                    .font(.system(size: 11, weight: .bold, design: .monospaced))
            }

            Divider().frame(height: 16)

            // Audio Meter
            AudioWaveformView(levels: speechManager.audioLevels)
                .frame(width: 40, height: 16)

            Divider().frame(height: 16)

            // Live Caption
            Text(speechManager.currentTranscript.isEmpty ? "Listening for director commentary..." : speechManager.currentTranscript)
                .font(.system(size: 11, weight: .medium))
                .foregroundColor(speechManager.currentTranscript.isEmpty ? .secondary : .primary)
                .lineLimit(1)
                .frame(maxWidth: 320, alignment: .leading)

            // Pause & Stop buttons
            Button(action: { captureManager.togglePause() }) {
                Image(systemName: captureManager.isPaused ? "play.fill" : "pause.fill")
            }
            .buttonStyle(.plain)

            Button(action: { captureManager.stopAndExport() }) {
                Image(systemName: "stop.fill")
                    .foregroundColor(.red)
            }
            .buttonStyle(.plain)
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 8)
        .background(.ultraThinMaterial)
        .clipShape(Capsule())
        .overlay(Capsule().stroke(Color.white.opacity(0.15), lineWidth: 1))
        .shadow(color: .black.opacity(0.25), radius: 8, x: 0, y: 4)
    }
}
```
