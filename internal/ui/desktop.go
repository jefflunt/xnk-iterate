package ui

import (
	"fmt"
	"sync"

	webview "github.com/webview/webview_go"
)

// DesktopApp manages the floating XNK Iterate Companion HUD window.
type DesktopApp struct {
	mu       sync.Mutex
	w        webview.WebView
	title    string
	url      string
	expanded bool
}

// NewDesktopApp creates the native OS floating Companion HUD window pointing at url.
func NewDesktopApp(title, url string) *DesktopApp {
	return &DesktopApp{
		title: title,
		url:   url,
	}
}

// SetDrawerExpanded resizes the floating HUD between its compact bar (960x108)
// and expanded Turns/Settings drawer (960x500).
func (a *DesktopApp) SetDrawerExpanded(expanded bool) {
	a.mu.Lock()
	w := a.w
	a.expanded = expanded
	a.mu.Unlock()

	if w == nil {
		return
	}
	w.Dispatch(func() {
		if expanded {
			w.SetSize(960, 500, webview.HintNone)
		} else {
			w.SetSize(960, 108, webview.HintNone)
		}
		configureNativeWindow(w.Window(), true)
	})
}

// Run opens the native floating HUD window and blocks on the main GUI thread until closed.
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
	w.SetSize(960, 108, webview.HintNone)
	w.SetSize(780, 96, webview.HintMin)

	// Always-on-top floating companion bar, self-excluded from screen captures on macOS
	configureNativeWindow(w.Window(), true)

	w.Navigate(a.url)
	w.Run()
	return nil
}
