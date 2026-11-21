//go:build !darwin

package ui

import "unsafe"

func configureNativeWindow(windowPtr unsafe.Pointer, floatingCompact bool) {
	// Cross-platform default: standard OS window handled by webview_go (WebView2 / WebKitGTK).
}
