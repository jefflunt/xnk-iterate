//go:build linux

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

type linuxCapturer struct {
	mu        sync.Mutex
	ffmpegCmd *exec.Cmd
	cancel    context.CancelFunc
	running   bool
}

// NewPlatformCapturer creates the Linux x11grab/ffmpeg capturer.
func NewPlatformCapturer() Capturer {
	return &linuxCapturer{}
}

func (l *linuxCapturer) ScreenRecordingStatus() string { return "granted" }

func (l *linuxCapturer) ListTargets() ([]model.CaptureTarget, error) {
	display := os.Getenv("DISPLAY")
	if display == "" {
		display = ":0.0"
	}
	return []model.CaptureTarget{
		{
			ID:         "display:0",
			Type:       model.TargetDisplay,
			Title:      "Primary Display (" + display + ")",
			AppName:    "X11 Display",
			Resolution: model.Size{Width: 1920, Height: 1080},
		},
	}, nil
}

func (l *linuxCapturer) Start(ctx context.Context, cfg Config) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.running {
		return nil
	}
	if err := os.MkdirAll(cfg.OutputDir, 0o755); err != nil {
		return err
	}
	runCtx, cancel := context.WithCancel(ctx)
	l.cancel = cancel
	l.running = true

	if FFmpegAvailable() && cfg.VideoFile != "" {
		fps := cfg.FPS
		if fps <= 0 {
			fps = 30
		}
		display := os.Getenv("DISPLAY")
		if display == "" {
			display = ":0.0"
		}
		videoPath := filepath.Join(cfg.OutputDir, cfg.VideoFile)
		cmd := exec.CommandContext(runCtx, "ffmpeg",
			"-y", "-f", "x11grab", "-framerate", strconv.Itoa(fps), "-i", display,
			"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p",
			"-movflags", "frag_keyframe+empty_moov+default_base_moof",
			videoPath,
		)
		if err := cmd.Start(); err == nil {
			l.ffmpegCmd = cmd
			go func() { _ = cmd.Wait() }()
		}
	}
	return nil
}

func (l *linuxCapturer) GrabSnapshot(ctx context.Context, target model.CaptureTarget) ([]byte, string, error) {
	return nil, "", nil
}

func (l *linuxCapturer) Stop() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.cancel != nil {
		l.cancel()
		l.cancel = nil
	}
	l.ffmpegCmd = nil
	l.running = false
	return nil
}
