package model

import (
	"fmt"
	"strings"
)

type keySpec struct {
	carbonCode uint32
	label      string
	isFuncKey  bool
}

// jsCodeToCarbon maps standard DOM KeyboardEvent.code values to macOS Carbon kVK_* virtual key codes.
var jsCodeToCarbon = map[string]keySpec{
	"KeyA": {0x00, "A", false},
	"KeyS": {0x01, "S", false},
	"KeyD": {0x02, "D", false},
	"KeyF": {0x03, "F", false},
	"KeyH": {0x04, "H", false},
	"KeyG": {0x05, "G", false},
	"KeyZ": {0x06, "Z", false},
	"KeyX": {0x07, "X", false},
	"KeyC": {0x08, "C", false},
	"KeyV": {0x09, "V", false},
	"KeyB": {0x0B, "B", false},
	"KeyQ": {0x0C, "Q", false},
	"KeyW": {0x0D, "W", false},
	"KeyE": {0x0E, "E", false},
	"KeyR": {0x0F, "R", false},
	"KeyY": {0x10, "Y", false},
	"KeyT": {0x11, "T", false},
	"KeyO": {0x1F, "O", false},
	"KeyU": {0x20, "U", false},
	"KeyI": {0x22, "I", false},
	"KeyP": {0x23, "P", false},
	"KeyL": {0x25, "L", false},
	"KeyJ": {0x26, "J", false},
	"KeyK": {0x28, "K", false},
	"KeyN": {0x2D, "N", false},
	"KeyM": {0x2E, "M", false},

	"Digit1": {0x12, "1", false},
	"Digit2": {0x13, "2", false},
	"Digit3": {0x14, "3", false},
	"Digit4": {0x15, "4", false},
	"Digit5": {0x17, "5", false},
	"Digit6": {0x16, "6", false},
	"Digit7": {0x1A, "7", false},
	"Digit8": {0x1C, "8", false},
	"Digit9": {0x19, "9", false},
	"Digit0": {0x1D, "0", false},

	"Enter":        {0x24, "↩", false},
	"NumpadEnter":  {0x4C, "⌤", false},
	"Backspace":    {0x33, "⌫", false},
	"Delete":       {0x75, "⌦", false},
	"Tab":          {0x30, "⇥", false},
	"Space":        {0x31, "Space", false},
	"Backquote":    {0x32, "`", false},
	"Minus":        {0x1B, "-", false},
	"Equal":        {0x18, "=", false},
	"BracketLeft":  {0x21, "[", false},
	"BracketRight": {0x1E, "]", false},
	"Backslash":    {0x2A, "\\", false},
	"Semicolon":    {0x29, ";", false},
	"Quote":        {0x27, "'", false},
	"Comma":        {0x2B, ",", false},
	"Period":       {0x2F, ".", false},
	"Slash":        {0x2C, "/", false},

	"ArrowLeft":  {0x7B, "←", false},
	"ArrowRight": {0x7C, "→", false},
	"ArrowDown":  {0x7D, "↓", false},
	"ArrowUp":    {0x7E, "↑", false},

	"F1":  {0x7A, "F1", true},
	"F2":  {0x78, "F2", true},
	"F3":  {0x63, "F3", true},
	"F4":  {0x76, "F4", true},
	"F5":  {0x60, "F5", true},
	"F6":  {0x61, "F6", true},
	"F7":  {0x62, "F7", true},
	"F8":  {0x64, "F8", true},
	"F9":  {0x65, "F9", true},
	"F10": {0x6D, "F10", true},
	"F11": {0x67, "F11", true},
	"F12": {0x6F, "F12", true},
}

// ResolveCarbonKeyCode converts a DOM KeyboardEvent.code string into its macOS Carbon virtual key code.
func ResolveCarbonKeyCode(jsCode string) (uint32, bool) {
	spec, ok := jsCodeToCarbon[jsCode]
	return spec.carbonCode, ok
}

// NormalizeHotkeyBinding validates a HotkeyBinding and computes its canonical macOS display label.
func NormalizeHotkeyBinding(b HotkeyBinding) (HotkeyBinding, error) {
	spec, ok := jsCodeToCarbon[b.KeyCode]
	if !ok {
		return b, fmt.Errorf("unsupported hotkey %q", b.KeyCode)
	}
	hasPrimaryMod := (b.Modifiers & (HotkeyModCmd | HotkeyModOption | HotkeyModControl)) != 0
	if !spec.isFuncKey && !hasPrimaryMod {
		return b, fmt.Errorf("shortcut %q requires ⌘, ⌥, or ⌃ modifier (or use F1–F12)", spec.label)
	}
	var sb strings.Builder
	if b.Modifiers&HotkeyModControl != 0 {
		sb.WriteString("⌃")
	}
	if b.Modifiers&HotkeyModOption != 0 {
		sb.WriteString("⌥")
	}
	if b.Modifiers&HotkeyModShift != 0 {
		sb.WriteString("⇧")
	}
	if b.Modifiers&HotkeyModCmd != 0 {
		sb.WriteString("⌘")
	}
	sb.WriteString(spec.label)
	b.Display = sb.String()
	return b, nil
}
