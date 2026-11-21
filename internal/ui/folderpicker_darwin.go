//go:build darwin

package ui

/*
#cgo LDFLAGS: -framework Cocoa
#include <stdlib.h>

char *xnkPickFolder(const char *initialDir);
*/
import "C"
import "unsafe"

// PickFolder opens the native macOS NSOpenPanel directory picker starting at initialDir.
// Returns the chosen directory path and true, or ("", false) if cancelled.
func PickFolder(initialDir string) (string, bool) {
	var cInit *C.char
	if initialDir != "" {
		cInit = C.CString(initialDir)
		defer C.free(unsafe.Pointer(cInit))
	}
	res := C.xnkPickFolder(cInit)
	if res == nil {
		return "", false
	}
	defer C.free(unsafe.Pointer(res))
	return C.GoString(res), true
}
