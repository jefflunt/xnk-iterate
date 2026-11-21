package ui

import (
	"fmt"
	"sync"

	webview "github.com/webview/webview_go"
)

// DesktopApp manages the cross-platform native WebView desktop window.
type DesktopApp struct {
	mu      sync.Mutex
	w       webview.WebView
	title   string
	url     string
	compact bool
}

// NewDesktopApp creates a native OS desktop window pointing at url.
func NewDesktopApp(title, url string) *DesktopApp {
	return &DesktopApp{
		title: title,
		url:   url,
	}
}

// SetCompactHUD switches the native window between full Director Studio and Compact Floating HUD.
func (a *DesktopApp) SetCompactHUD(compact bool) {
	a.mu.Lock()
	w := a.w
	a.compact = compact
	a.mu.Unlock()

	if w == nil {
		return
	}
	w.Dispatch(func() {
		if compact {
			w.SetSize(900, 148, webview.HintNone)
			configureNativeWindow(w.Window(), true)
		} else {
			w.SetSize(1120, 740, webview.HintNone)
			configureNativeWindow(w.Window(), false)
		}
	})
}

// Run opens the native OS window and blocks on the main GUI thread until closed.
func (a *DesktopApp) Run() error {
	w := webview.New(true)
	if w == nil {
		return fmt.Errorf("failed to initialize native webview window")
	}

	a.mu.Lock()
	a.w = w
	a.mu.Unlock()

	defer func() {
		a.mu.Lock()
		a.w = nil
		a.mu.Unlock()
		w.Destroy()
	}()

	w.SetTitle(a.title)
	w.SetSize(1120, 740, webview.HintNone)
	w.SetSize(820, 130, webview.HintMin)

	// Apply platform-specific native window styling (e.g., macOS capture self-exclusion)
	configureNativeWindow(w.Window(), false)

	w.Navigate(a.url)
	w.Run()
	return nil
}
