//go:build !darwin

package ui

import "github.com/jefflunt/xnk-iterate/internal/model"

const (
	HotkeySendTurn    = 1
	HotkeyDiscardTurn = 2
)

// RegisterGlobalHotkeys is a no-op stub on non-macOS platforms.
func RegisterGlobalHotkeys(cfg model.HotkeyConfig, onPress func(hotkeyID int)) {}

// UpdateGlobalHotkeys is a no-op stub on non-macOS platforms.
func UpdateGlobalHotkeys(cfg model.HotkeyConfig) {}

// SuspendGlobalHotkeys is a no-op stub on non-macOS platforms.
func SuspendGlobalHotkeys(suspended bool) {}

// UnregisterGlobalHotkeys is a no-op stub on non-macOS platforms.
func UnregisterGlobalHotkeys() {}
