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

// SetDrawerExpanded resizes the floating HUD between its borderless compact bar (960x88)
// and expanded Turns/Settings drawer (960x490).
func (a *DesktopApp) SetDrawerExpanded(expanded bool) {
	a.mu.Lock()
	w := a.w
	a.expanded = expanded
	a.mu.Unlock()

	if w == nil {
		return
	}
	w.Dispatch(func() {
		configureNativeWindow(w.Window(), true)
		if expanded {
			w.SetSize(960, 490, webview.HintNone)
		} else {
			w.SetSize(960, 88, webview.HintNone)
		}
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

	// Always-on-top borderless floating companion bar, self-excluded from screen captures on macOS.
	// Configure FullSizeContentView BEFORE sizing so macOS allocates the full height without clipping.
	configureNativeWindow(w.Window(), true)

	w.SetTitle(a.title)
	w.SetSize(960, 88, webview.HintNone)
	w.SetSize(760, 84, webview.HintMin)

	// Bind native Cocoa window drag for the borderless HUD (drag from XNK badge or bar background)
	_ = w.Bind("__xnkStartDrag", func() {
		startNativeWindowDrag(w.Window())
	})

	w.Navigate(a.url)
	w.Run()
	return nil
}
