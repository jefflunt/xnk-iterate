//go:build !darwin

package ui

import "github.com/jefflunt/xnk-iterate/internal/model"

type fallbackCaptureBox struct {
	onChange func(model.CaptureRegion)
}

// NewPlatformCaptureBox returns a cross-platform fallback capture box controller.
// Windows (WS_EX_LAYERED | WS_EX_TRANSPARENT) and Linux (X11/Wayland overlay) plug in here.
func NewPlatformCaptureBox() CaptureBoxController {
	return &fallbackCaptureBox{}
}

func (f *fallbackCaptureBox) Show(region model.CaptureRegion)                     {}
func (f *fallbackCaptureBox) Hide()                                               {}
func (f *fallbackCaptureBox) UpdateRegion(region model.CaptureRegion)             {}
func (f *fallbackCaptureBox) SetLocked(locked bool)                               {}
func (f *fallbackCaptureBox) SetOnRegionChange(fn func(model.CaptureRegion))      { f.onChange = fn }
func (f *fallbackCaptureBox) Destroy()                                            {}
