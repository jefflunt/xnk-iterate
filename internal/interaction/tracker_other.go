//go:build !darwin

package interaction

type noopTracker struct{}

// NewPlatformTracker returns a cross-platform fallback tracker.
// Windows (SetWindowsHookEx) and Linux (XRecord/evdev) hooks plug in here.
func NewPlatformTracker() Tracker {
	return &noopTracker{}
}

func (n *noopTracker) AccessibilityStatus() string { return "unknown" }
func (n *noopTracker) Start(elapsedFn func() float64, timeline *Timeline) error {
	return nil
}
func (n *noopTracker) Stop() {}
