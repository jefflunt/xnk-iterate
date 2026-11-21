//go:build darwin

package ui

/*
#cgo LDFLAGS: -framework Cocoa
#include <stdbool.h>

void setupMacMainMenu(void);
void setMacAppIcon(const void *pngBytes, int length);
void configureMacWindow(void *winPtr, bool floatingCompact);
*/
import "C"
import (
	_ "embed"
	"sync"
	"unsafe"
)

//go:embed assets/icon.png
var embeddedAppIconPNG []byte

var (
	appIconOnce    sync.Once
	terminateMu    sync.Mutex
	terminateOnce  sync.Once
	onAppTerminate func()
)

// SetOnTerminate registers a cleanup hook invoked once when the main window closes or Cmd+Q is pressed.
func SetOnTerminate(fn func()) {
	terminateMu.Lock()
	onAppTerminate = fn
	terminateMu.Unlock()
}

//export goAppWillTerminate
func goAppWillTerminate() {
	terminateOnce.Do(func() {
		terminateMu.Lock()
		cb := onAppTerminate
		terminateMu.Unlock()
		if cb != nil {
			cb()
		}
	})
}

func configureNativeWindow(windowPtr unsafe.Pointer, floatingCompact bool) {
	appIconOnce.Do(func() {
		C.setupMacMainMenu()
		if len(embeddedAppIconPNG) > 0 {
			C.setMacAppIcon(unsafe.Pointer(&embeddedAppIconPNG[0]), C.int(len(embeddedAppIconPNG)))
		}
	})
	if windowPtr == nil {
		return
	}
	C.configureMacWindow(windowPtr, C.bool(floatingCompact))
}
