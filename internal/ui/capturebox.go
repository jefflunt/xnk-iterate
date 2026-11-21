package ui

import "github.com/jefflunt/xnk-iterate/internal/model"

// CaptureBoxController manages the semi-detachable, multi-monitor native screen region frame.
// When detached, it floats over the desktop across any monitor; when locked during recording,
// it freezes position/size and becomes click-through so the user interacts with the app inside it.
type CaptureBoxController interface {
	Show(region model.CaptureRegion)
	Hide()
	UpdateRegion(region model.CaptureRegion)
	SetLocked(locked bool)
	SetOnRegionChange(fn func(region model.CaptureRegion))
	Destroy()
}
