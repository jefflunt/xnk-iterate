//go:build windows

package capture

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/jefflunt/xnk-iterate/internal/model"
)

type windowsCapturer struct {
	mu        sync.Mutex
	ffmpegCmd *exec.Cmd
	cancel    context.CancelFunc
	running   bool
}

// NewPlatformCapturer creates the Windows gdigrab/ffmpeg capturer.
func NewPlatformCapturer() Capturer {
	return &windowsCapturer{}
}

func (w *windowsCapturer) ScreenRecordingStatus() string { return "granted" }

func (w *windowsCapturer) ListTargets() ([]model.CaptureTarget, error) {
	return []model.CaptureTarget{
		{
			ID:         "display:0",
			Type:       model.TargetDisplay,
			Title:      "Primary Desktop (Windows gdigrab)",
			AppName:    "Windows Desktop",
			Resolution: model.Size{Width: 1920, Height: 1080},
		},
	}, nil
}

func (w *windowsCapturer) Start(ctx context.Context, cfg Config) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.running {
		return nil
	}
	if err := os.MkdirAll(cfg.OutputDir, 0o755); err != nil {
		return err
	}
	runCtx, cancel := context.WithCancel(ctx)
	w.cancel = cancel
	w.running = true

	if FFmpegAvailable() && cfg.VideoFile != "" {
		fps := cfg.FPS
		if fps <= 0 {
			fps = 30
		}
		videoPath := filepath.Join(cfg.OutputDir, cfg.VideoFile)
		cmd := exec.CommandContext(runCtx, "ffmpeg",
			"-y", "-f", "gdigrab", "-framerate", strconv.Itoa(fps), "-i", "desktop",
			"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p",
			"-movflags", "frag_keyframe+empty_moov+default_base_moof",
			videoPath,
		)
		if err := cmd.Start(); err == nil {
			w.ffmpegCmd = cmd
			go func() { _ = cmd.Wait() }()
		}
	}
	return nil
}

func (w *windowsCapturer) GrabSnapshot(ctx context.Context, target model.CaptureTarget) ([]byte, string, error) {
	return nil, "", nil
}

func (w *windowsCapturer) Stop() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.cancel != nil {
		w.cancel()
		w.cancel = nil
	}
	w.ffmpegCmd = nil
	w.running = false
	return nil
}
