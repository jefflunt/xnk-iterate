//go:build darwin

package capture

/*
#cgo LDFLAGS: -framework CoreGraphics -framework CoreFoundation -framework ApplicationServices
#include <CoreGraphics/CoreGraphics.h>
#include <CoreFoundation/CoreFoundation.h>
#include <stdlib.h>
#include <stdbool.h>

typedef struct {
    int id;
    int x;
    int y;
    int width;
    int height;
    char owner[256];
    char title[256];
} MacWindowInfo;

static bool hasScreenCaptureAccess(void) {
    return CGPreflightScreenCaptureAccess();
}

static int listMacDisplays(int *ids, int *widths, int *heights, int maxCount) {
    uint32_t count = 0;
    CGDirectDisplayID displays[16];
    if (maxCount > 16) maxCount = 16;
    if (CGGetActiveDisplayList((uint32_t)maxCount, displays, &count) != kCGErrorSuccess) {
        return 0;
    }
    for (uint32_t i = 0; i < count; i++) {
        ids[i] = (int)displays[i];
        widths[i] = (int)CGDisplayPixelsWide(displays[i]);
        heights[i] = (int)CGDisplayPixelsHigh(displays[i]);
    }
    return (int)count;
}

static int listMacWindows(MacWindowInfo *out, int maxCount) {
    CFArrayRef windowList = CGWindowListCopyWindowInfo(
        kCGWindowListOptionOnScreenOnly | kCGWindowListExcludeDesktopElements,
        kCGNullWindowID
    );
    if (!windowList) return 0;

    CFIndex total = CFArrayGetCount(windowList);
    int written = 0;
    for (CFIndex i = 0; i < total && written < maxCount; i++) {
        CFDictionaryRef dict = (CFDictionaryRef)CFArrayGetValueAtIndex(windowList, i);
        if (!dict) continue;

        int layer = 0;
        CFNumberRef layerRef = (CFNumberRef)CFDictionaryGetValue(dict, kCGWindowLayer);
        if (layerRef) CFNumberGetValue(layerRef, kCFNumberIntType, &layer);
        if (layer != 0) continue; // normal application windows only

        CGRect bounds = CGRectZero;
        CFDictionaryRef boundsDict = (CFDictionaryRef)CFDictionaryGetValue(dict, kCGWindowBounds);
        if (boundsDict) CGRectMakeWithDictionaryRepresentation(boundsDict, &bounds);
        if (bounds.size.width < 120 || bounds.size.height < 80) continue;

        int wid = 0;
        CFNumberRef idRef = (CFNumberRef)CFDictionaryGetValue(dict, kCGWindowNumber);
        if (idRef) CFNumberGetValue(idRef, kCFNumberIntType, &wid);

        MacWindowInfo info;
        memset(&info, 0, sizeof(info));
        info.id = wid;
        info.x = (int)bounds.origin.x;
        info.y = (int)bounds.origin.y;
        info.width = (int)bounds.size.width;
        info.height = (int)bounds.size.height;

        CFStringRef ownerRef = (CFStringRef)CFDictionaryGetValue(dict, kCGWindowOwnerName);
        if (ownerRef) {
            CFStringGetCString(ownerRef, info.owner, sizeof(info.owner), kCFStringEncodingUTF8);
        }
        CFStringRef nameRef = (CFStringRef)CFDictionaryGetValue(dict, kCGWindowName);
        if (nameRef) {
            CFStringGetCString(nameRef, info.title, sizeof(info.title), kCFStringEncodingUTF8);
        }

        out[written++] = info;
    }
    CFRelease(windowList);
    return written;
}
*/
import "C"

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jefflunt/xnk-iterate/internal/model"
)

type darwinCapturer struct {
	mu        sync.Mutex
	ffmpegCmd *exec.Cmd
	cancel    context.CancelFunc
	running   bool
}

// NewPlatformCapturer creates the macOS capturer using CoreGraphics enumeration,
// ffmpeg Fragmented MP4 recording, and rolling PNG ring-buffer frame extraction.
func NewPlatformCapturer() Capturer {
	return &darwinCapturer{}
}

func (d *darwinCapturer) ScreenRecordingStatus() string {
	if bool(C.hasScreenCaptureAccess()) {
		return "granted"
	}
	return "denied"
}

func (d *darwinCapturer) ListTargets() ([]model.CaptureTarget, error) {
	var targets []model.CaptureTarget

	// 1. Active Displays
	var ids [16]C.int
	var widths [16]C.int
	var heights [16]C.int
	dispCount := int(C.listMacDisplays(&ids[0], &widths[0], &heights[0], 16))
	for i := 0; i < dispCount; i++ {
		w, h := int(widths[i]), int(heights[i])
		if w <= 0 {
			w = 1920
		}
		if h <= 0 {
			h = 1080
		}
		targets = append(targets, model.CaptureTarget{
			ID:           fmt.Sprintf("display:%d", i),
			Type:         model.TargetDisplay,
			Title:        fmt.Sprintf("Display %d (%dx%d)", i+1, w, h),
			AppName:      "macOS Display",
			Resolution:   model.Size{Width: w, Height: h},
			Bounds:       model.Rect{X: 0, Y: 0, Width: w, Height: h},
			DisplayIndex: i,
		})
	}
	if len(targets) == 0 {
		targets = append(targets, model.CaptureTarget{
			ID:           "display:0",
			Type:         model.TargetDisplay,
			Title:        "Primary Display",
			AppName:      "macOS Display",
			Resolution:   model.Size{Width: 1920, Height: 1080},
			DisplayIndex: 0,
		})
	}

	// 2. Visible Application Windows
	var winInfos [64]C.MacWindowInfo
	winCount := int(C.listMacWindows(&winInfos[0], 64))
	for i := 0; i < winCount; i++ {
		w := winInfos[i]
		owner := strings.TrimSpace(C.GoString(&w.owner[0]))
		title := strings.TrimSpace(C.GoString(&w.title[0]))
		if owner == "" || owner == "xnk-iterate" {
			continue
		}
		displayTitle := owner
		if title != "" && title != owner {
			displayTitle = fmt.Sprintf("%s — %s", owner, title)
		}
		targets = append(targets, model.CaptureTarget{
			ID:      fmt.Sprintf("window:%d", int(w.id)),
			Type:    model.TargetWindow,
			Title:   displayTitle,
			AppName: owner,
			Resolution: model.Size{
				Width:  int(w.width),
				Height: int(w.height),
			},
			Bounds: model.Rect{
				X:      int(w.x),
				Y:      int(w.y),
				Width:  int(w.width),
				Height: int(w.height),
			},
		})
	}

	return targets, nil
}

func (d *darwinCapturer) Start(ctx context.Context, cfg Config) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.running {
		return nil
	}

	if err := os.MkdirAll(cfg.OutputDir, 0o755); err != nil {
		return err
	}

	runCtx, cancel := context.WithCancel(ctx)
	d.cancel = cancel
	d.running = true

	// 1. Start Fragmented MP4 recording via ffmpeg (if ffmpeg is installed & screen access available)
	if FFmpegAvailable() && cfg.VideoFile != "" {
		fps := cfg.FPS
		if fps <= 0 {
			fps = 30
		}
		videoPath := filepath.Join(cfg.OutputDir, cfg.VideoFile)
		dispIdx := cfg.Target.DisplayIndex
		if cfg.Region != nil && cfg.Region.DisplayIndex >= 0 {
			dispIdx = cfg.Region.DisplayIndex
		}
		inputSpec := fmt.Sprintf("Capture screen %d:none", dispIdx)
		args := []string{
			"-y",
			"-f", "avfoundation",
			"-framerate", strconv.Itoa(fps),
			"-capture_cursor", "1",
			"-i", inputSpec,
		}
		if cfg.Region != nil && cfg.Region.Width > 32 && cfg.Region.Height > 32 {
			// Ensure even dimensions for H.264 yuv420p encoding
			w := cfg.Region.Width &^ 1
			h := cfg.Region.Height &^ 1
			args = append(args, "-vf", fmt.Sprintf("crop=%d:%d:%d:%d", w, h, max(0, cfg.Region.X), max(0, cfg.Region.Y)))
		}
		args = append(args,
			"-c:v", "h264_videotoolbox",
			"-b:v", "6M",
			"-pix_fmt", "yuv420p",
			"-movflags", "frag_keyframe+empty_moov+default_base_moof",
			"-frag_duration", "1000000",
			videoPath,
		)
		cmd := exec.CommandContext(runCtx, "ffmpeg", args...)
		if err := cmd.Start(); err == nil {
			d.ffmpegCmd = cmd
			go func() {
				_ = cmd.Wait()
			}()
		}
	}

	// 2. Start rolling in-memory frame ring buffer feeder (~2 FPS) for instant Trigger/Reaction snapshots
	go d.runRingBufferFeeder(runCtx, cfg)

	return nil
}

func (d *darwinCapturer) runRingBufferFeeder(ctx context.Context, cfg Config) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	grabOnce := func() {
		if cfg.RingBuffer == nil {
			return
		}
		data, format, err := d.GrabSnapshot(ctx, cfg.Target, cfg.Region)
		if err != nil || len(data) == 0 {
			return
		}
		ts := 0.0
		if cfg.ElapsedFn != nil {
			ts = cfg.ElapsedFn()
		}
		cfg.RingBuffer.Push(ts, format, data)
		if cfg.OnLiveFrame != nil {
			cfg.OnLiveFrame(ts, format, data)
		}
	}

	// Capture an immediate initial frame
	grabOnce()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			grabOnce()
		}
	}
}

func (d *darwinCapturer) GrabSnapshot(ctx context.Context, target model.CaptureTarget, region *model.CaptureRegion) ([]byte, string, error) {
	tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("xnk_snap_%d.jpg", time.Now().UnixNano()))
	defer os.Remove(tmpFile)

	args := []string{"-x", "-t", "jpg"}
	if region != nil && region.Width > 32 && region.Height > 32 {
		// Global screen-space rectangle across all monitors!
		args = append(args, "-R", fmt.Sprintf("%d,%d,%d,%d", region.X, region.Y, region.Width, region.Height))
	} else if target.Type == model.TargetWindow && strings.HasPrefix(target.ID, "window:") {
		wid := strings.TrimPrefix(target.ID, "window:")
		args = append(args, "-l", wid)
	} else {
		args = append(args, "-D", strconv.Itoa(target.DisplayIndex+1))
	}
	args = append(args, tmpFile)

	snapCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(snapCtx, "screencapture", args...)
	if err := cmd.Run(); err != nil {
		return nil, "", err
	}
	data, err := os.ReadFile(tmpFile)
	if err != nil {
		return nil, "", err
	}
	return data, "jpg", nil
}

func (d *darwinCapturer) Stop() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.running {
		return nil
	}
	if d.ffmpegCmd != nil && d.ffmpegCmd.Process != nil {
		// Send SIGINT so ffmpeg finalizes the fMP4 container cleanly before context cancel.
		_ = d.ffmpegCmd.Process.Signal(os.Interrupt)
		time.Sleep(250 * time.Millisecond)
		d.ffmpegCmd = nil
	}
	if d.cancel != nil {
		d.cancel()
		d.cancel = nil
	}
	d.running = false
	return nil
}
