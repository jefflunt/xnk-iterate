package ui

import (
	"fmt"
	"sync"

	webview "github.com/webview/webview_go"
)

const (
	DefaultWindowWidth    = 960
	MinWindowWidth        = 760
	MaxWindowWidth        = 4096
	CollapsedWindowHeight = 72
	ExpandedWindowHeight  = 480
)

// NormalizeWindowWidth clamps a persisted or reported HUD width to valid bounds,
// falling back to DefaultWindowWidth when unset (<= 0).
func NormalizeWindowWidth(width int) int {
	if width <= 0 {
		return DefaultWindowWidth
	}
	if width < MinWindowWidth {
		return MinWindowWidth
	}
	if width > MaxWindowWidth {
		return MaxWindowWidth
	}
	return width
}

// DesktopApp manages the floating XNK Iterate Companion HUD window.
type DesktopApp struct {
	mu             sync.Mutex
	w              webview.WebView
	title          string
	url            string
	expanded       bool
	width          int
	lastSavedWidth int
	onWidthChange  func(width int)
}

// NewDesktopApp creates the native OS floating Companion HUD window pointing at url.
func NewDesktopApp(title, url string) *DesktopApp {
	return &DesktopApp{
		title:          title,
		url:            url,
		width:          DefaultWindowWidth,
		lastSavedWidth: DefaultWindowWidth,
	}
}

// SetInitialWidth sets the starting width of the HUD before Run() opens the window.
func (a *DesktopApp) SetInitialWidth(width int) {
	w := NormalizeWindowWidth(width)
	a.mu.Lock()
	a.width = w
	a.lastSavedWidth = w
	a.mu.Unlock()
}

// SetOnWidthChange registers a callback invoked when the user finishes resizing the HUD horizontally.
func (a *DesktopApp) SetOnWidthChange(fn func(width int)) {
	a.mu.Lock()
	a.onWidthChange = fn
	a.mu.Unlock()
}

func (a *DesktopApp) currentWidth() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return NormalizeWindowWidth(a.width)
}

func (a *DesktopApp) handleNativeResize(width int, inLiveResize bool) {
	w := NormalizeWindowWidth(width)
	var cb func(int)

	a.mu.Lock()
	a.width = w
	if !inLiveResize && w != a.lastSavedWidth && a.onWidthChange != nil {
		a.lastSavedWidth = w
		cb = a.onWidthChange
	}
	a.mu.Unlock()

	if cb != nil {
		go cb(w)
	}
}

// SetDrawerExpanded resizes the floating HUD between its borderless compact bar (width x 72)
// and expanded Turns/Settings drawer (width x 480), preserving the user's current width and
// keeping vertical resizing locked.
func (a *DesktopApp) SetDrawerExpanded(expanded bool) {
	a.mu.Lock()
	w := a.w
	a.expanded = expanded
	width := NormalizeWindowWidth(a.width)
	a.mu.Unlock()

	if w == nil {
		return
	}
	targetHeight := CollapsedWindowHeight
	if expanded {
		targetHeight = ExpandedWindowHeight
	}
	w.Dispatch(func() {
		w.SetSize(width, targetHeight, webview.HintNone)
		configureNativeWindow(w.Window(), true, MinWindowWidth, width, targetHeight)
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
	initialWidth := NormalizeWindowWidth(a.width)
	a.mu.Unlock()

	setNativeWindowResizeHandler(a.handleNativeResize)

	defer func() {
		setNativeWindowResizeHandler(nil)
		a.mu.Lock()
		a.w = nil
		finalWidth := NormalizeWindowWidth(a.width)
		var cb func(int)
		if finalWidth != a.lastSavedWidth && a.onWidthChange != nil {
			a.lastSavedWidth = finalWidth
			cb = a.onWidthChange
		}
		a.mu.Unlock()
		if cb != nil {
			cb(finalWidth)
		}
		w.Destroy()
	}()

	w.SetTitle(a.title)
	w.SetSize(initialWidth, CollapsedWindowHeight, webview.HintNone)
	w.SetSize(MinWindowWidth, CollapsedWindowHeight, webview.HintMin)

	// Bind native Cocoa window drag for the borderless HUD (drag from XNK badge or bar background)
	_ = w.Bind("__xnkStartDrag", func() {
		startNativeWindowDrag(w.Window())
	})

	// Configure borderless FullSizeContentView, lock vertical resizing, and apply persisted width
	// AFTER webview_go's SetSize (because SetSize resets NSWindow styleMask and size constraints on macOS).
	configureNativeWindow(w.Window(), true, MinWindowWidth, initialWidth, CollapsedWindowHeight)

	w.Navigate(a.url)
	w.Run()
	return nil
}
