//go:build darwin

package ui

/*
#cgo LDFLAGS: -framework Carbon -framework Cocoa
#include <stdint.h>

void xnkUpdateGlobalHotkeys(uint32_t sendKeyCode, uint32_t sendModMask, uint32_t discardKeyCode, uint32_t discardModMask);
void xnkSuspendGlobalHotkeys(int suspended);
void xnkUnregisterGlobalHotkeys(void);
*/
import "C"
import (
	"sync"

	"github.com/jefflunt/xnk-iterate/internal/model"
)

const (
	HotkeySendTurn    = 1
	HotkeyDiscardTurn = 2
)

var (
	hotkeyMu sync.RWMutex
	hotkeyCB func(hotkeyID int)
)

//export goGlobalHotkeyPressed
func goGlobalHotkeyPressed(hotkeyID C.int) {
	hotkeyMu.RLock()
	cb := hotkeyCB
	hotkeyMu.RUnlock()
	if cb != nil {
		go cb(int(hotkeyID))
	}
}

// RegisterGlobalHotkeys sets the callback and registers the initial global macOS Carbon hotkeys.
func RegisterGlobalHotkeys(cfg model.HotkeyConfig, onPress func(hotkeyID int)) {
	hotkeyMu.Lock()
	hotkeyCB = onPress
	hotkeyMu.Unlock()
	UpdateGlobalHotkeys(cfg)
}

// UpdateGlobalHotkeys dynamically re-registers the global macOS Carbon hotkeys without restarting.
func UpdateGlobalHotkeys(cfg model.HotkeyConfig) {
	sendCode, ok := model.ResolveCarbonKeyCode(cfg.SendTurn.KeyCode)
	if !ok {
		sendCode = 0x24 // kVK_Return
	}
	discardCode, ok := model.ResolveCarbonKeyCode(cfg.DiscardTurn.KeyCode)
	if !ok {
		discardCode = 0x33 // kVK_Delete
	}
	C.xnkUpdateGlobalHotkeys(
		C.uint32_t(sendCode),
		C.uint32_t(cfg.SendTurn.Modifiers),
		C.uint32_t(discardCode),
		C.uint32_t(cfg.DiscardTurn.Modifiers),
	)
}

// SuspendGlobalHotkeys temporarily unregisters global hotkeys while the user records a new shortcut in Settings.
func SuspendGlobalHotkeys(suspended bool) {
	if suspended {
		C.xnkSuspendGlobalHotkeys(1)
	} else {
		C.xnkSuspendGlobalHotkeys(0)
	}
}

// UnregisterGlobalHotkeys removes registered system-wide hotkeys cleanly on exit.
func UnregisterGlobalHotkeys() {
	hotkeyMu.Lock()
	hotkeyCB = nil
	hotkeyMu.Unlock()
	C.xnkUnregisterGlobalHotkeys()
}
