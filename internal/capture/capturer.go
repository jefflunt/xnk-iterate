package capture

import (
	"context"
	"os/exec"

	"github.com/jefflunt/xnk-iterate/internal/model"
	"github.com/jefflunt/xnk-iterate/internal/ringbuffer"
)

// Config defines parameters for starting a video/audio capture session.
type Config struct {
	Target      model.CaptureTarget
	OutputDir   string
	VideoFile   string
	FPS         int
	RingBuffer  *ringbuffer.FrameRingBuffer
	ElapsedFn   func() float64
	OnLiveFrame func(ts float64, format string, data []byte)
}

// Capturer is the cross-platform screen and audio capture interface.
type Capturer interface {
	ListTargets() ([]model.CaptureTarget, error)
	Start(ctx context.Context, cfg Config) error
	Stop() error
	GrabSnapshot(ctx context.Context, target model.CaptureTarget) ([]byte, string, error)
	ScreenRecordingStatus() string
}

// FFmpegAvailable reports whether ffmpeg is discoverable on PATH.
func FFmpegAvailable() bool {
	_, err := exec.LookPath("ffmpeg")
	return err == nil
}
