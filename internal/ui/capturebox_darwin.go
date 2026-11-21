//go:build darwin

package ui

/*
#cgo LDFLAGS: -framework Cocoa
#include <stdbool.h>

void xnkShowCaptureBox(int x, int y, int w, int h, bool locked);
void xnkHideCaptureBox(void);
void xnkSetCaptureBoxLocked(bool locked);
void xnkDestroyCaptureBox(void);
*/
import "C"

import (
	"sync"

	"github.com/jefflunt/xnk-iterate/internal/model"
)

var (
	boxCallbackMu sync.RWMutex
	boxCallback   func(model.CaptureRegion)
)

//export goCaptureBoxMoved
func goCaptureBoxMoved(x, y, w, h, displayIdx C.int) {
	boxCallbackMu.RLock()
	cb := boxCallback
	boxCallbackMu.RUnlock()
	if cb != nil {
		cb(model.CaptureRegion{
			X:            int(x),
			Y:            int(y),
			Width:        int(w),
			Height:       int(h),
			DisplayIndex: int(displayIdx),
			Detached:     true,
		})
	}
}

type darwinCaptureBox struct {
	mu     sync.Mutex
	region model.CaptureRegion
}

// NewPlatformCaptureBox creates the macOS transparent, multi-monitor, click-through-when-locked Capture Box window.
func NewPlatformCaptureBox() CaptureBoxController {
	return &darwinCaptureBox{}
}

func (d *darwinCaptureBox) SetOnRegionChange(fn func(model.CaptureRegion)) {
	boxCallbackMu.Lock()
	boxCallback = fn
	boxCallbackMu.Unlock()
}

func (d *darwinCaptureBox) Show(region model.CaptureRegion) {
	d.mu.Lock()
	d.region = region
	d.mu.Unlock()
	C.xnkShowCaptureBox(C.int(region.X), C.int(region.Y), C.int(region.Width), C.int(region.Height), C.bool(region.Locked))
}

func (d *darwinCaptureBox) Hide() {
	C.xnkHideCaptureBox()
}

func (d *darwinCaptureBox) UpdateRegion(region model.CaptureRegion) {
	d.mu.Lock()
	d.region = region
	d.mu.Unlock()
	if region.Detached {
		C.xnkShowCaptureBox(C.int(region.X), C.int(region.Y), C.int(region.Width), C.int(region.Height), C.bool(region.Locked))
	}
}

func (d *darwinCaptureBox) SetLocked(locked bool) {
	d.mu.Lock()
	d.region.Locked = locked
	d.mu.Unlock()
	C.xnkSetCaptureBoxLocked(C.bool(locked))
}

func (d *darwinCaptureBox) Destroy() {
	C.xnkDestroyCaptureBox()
}
