//go:build darwin

package ui

/*
#cgo LDFLAGS: -framework Cocoa
#include <stdbool.h>

void setupMacMainMenu(void);
void setMacAppIcon(const void *pngBytes, int length);
void configureMacWindow(void *winPtr, bool floatingCompact, int minWidth, int targetWidth, int targetHeight);
void startMacWindowDrag(void *winPtr);
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

	resizeMu      sync.Mutex
	onWindowResize func(width int, inLiveResize bool)
)

// SetOnTerminate registers a cleanup hook invoked once when the main window closes or Cmd+Q is pressed.
func SetOnTerminate(fn func()) {
	terminateMu.Lock()
	onAppTerminate = fn
	terminateMu.Unlock()
}

func setNativeWindowResizeHandler(fn func(width int, inLiveResize bool)) {
	resizeMu.Lock()
	onWindowResize = fn
	resizeMu.Unlock()
}

//export goWindowDidResize
func goWindowDidResize(width C.int, inLiveResize C.bool) {
	resizeMu.Lock()
	cb := onWindowResize
	resizeMu.Unlock()
	if cb != nil {
		cb(int(width), bool(inLiveResize))
	}
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

func configureNativeWindow(windowPtr unsafe.Pointer, floatingCompact bool, minWidth, width, height int) {
	appIconOnce.Do(func() {
		C.setupMacMainMenu()
		if len(embeddedAppIconPNG) > 0 {
			C.setMacAppIcon(unsafe.Pointer(&embeddedAppIconPNG[0]), C.int(len(embeddedAppIconPNG)))
		}
	})
	if windowPtr == nil {
		return
	}
	C.configureMacWindow(windowPtr, C.bool(floatingCompact), C.int(minWidth), C.int(width), C.int(height))
}

func startNativeWindowDrag(windowPtr unsafe.Pointer) {
	if windowPtr == nil {
		return
	}
	C.startMacWindowDrag(windowPtr)
}
