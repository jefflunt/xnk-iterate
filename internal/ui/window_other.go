//go:build !darwin

package ui

import "unsafe"

// SetOnTerminate is a no-op stub on non-macOS platforms.
func SetOnTerminate(fn func()) {}

func configureNativeWindow(windowPtr unsafe.Pointer, floatingCompact bool) {
	// Cross-platform default: standard OS window handled by webview_go (WebView2 / WebKitGTK).
}

func startNativeWindowDrag(windowPtr unsafe.Pointer) {}
