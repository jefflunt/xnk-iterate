//go:build darwin

package interaction

/*
#cgo LDFLAGS: -framework ApplicationServices -framework CoreFoundation -framework CoreGraphics -framework Cocoa
#include <ApplicationServices/ApplicationServices.h>
#include <CoreGraphics/CoreGraphics.h>
#include <stdbool.h>

extern void goInteractionCallback(int evType, double x, double y, int keyCode);

static CFMachPortRef gEventTap = NULL;
static CFRunLoopRef gTapRunLoop = NULL;

static CGEventRef eventTapCallback(CGEventTapProxy proxy, CGEventType type, CGEventRef event, void *refcon) {
    if (type == kCGEventTapDisabledByTimeout || type == kCGEventTapDisabledByUserInput) {
        if (gEventTap != NULL) {
            CGEventTapEnable(gEventTap, true);
        }
        return event;
    }

    CGPoint loc = CGEventGetLocation(event);
    int keyCode = 0;
    int mapped = 0;
    switch (type) {
        case kCGEventLeftMouseDown:
            mapped = 1;
            break;
        case kCGEventRightMouseDown:
            mapped = 2;
            break;
        case kCGEventKeyDown:
            mapped = 3;
            keyCode = (int)CGEventGetIntegerValueField(event, kCGKeyboardEventKeycode);
            break;
        default:
            return event;
    }
    goInteractionCallback(mapped, loc.x, loc.y, keyCode);
    return event;
}

static bool isAccessibilityTrusted(void) {
    return AXIsProcessTrusted();
}

static int startMacEventTap(void) {
    if (!AXIsProcessTrusted()) {
        return 0;
    }
    CGEventMask mask = CGEventMaskBit(kCGEventLeftMouseDown) |
                       CGEventMaskBit(kCGEventRightMouseDown) |
                       CGEventMaskBit(kCGEventKeyDown);
    gEventTap = CGEventTapCreate(
        kCGSessionEventTap,
        kCGHeadInsertEventTap,
        kCGEventTapOptionListenOnly,
        mask,
        eventTapCallback,
        NULL
    );
    if (gEventTap == NULL) {
        return 0;
    }
    CFRunLoopSourceRef source = CFMachPortCreateRunLoopSource(kCFAllocatorDefault, gEventTap, 0);
    gTapRunLoop = CFRunLoopGetCurrent();
    CFRunLoopAddSource(gTapRunLoop, source, kCFRunLoopCommonModes);
    CGEventTapEnable(gEventTap, true);
    CFRelease(source);
    CFRunLoopRun();
    return 1;
}

static void stopMacEventTap(void) {
    if (gEventTap != NULL) {
        CGEventTapEnable(gEventTap, false);
        CFRelease(gEventTap);
        gEventTap = NULL;
    }
    if (gTapRunLoop != NULL) {
        CFRunLoopStop(gTapRunLoop);
        gTapRunLoop = NULL;
    }
}
*/
import "C"

import (
	"fmt"
	"runtime"
	"sync"

	"github.com/jefflunt/xnk-iterate/internal/model"
)

var (
	activeMu       sync.RWMutex
	activeTimeline *Timeline
	activeElapsed  func() float64
)

//export goInteractionCallback
func goInteractionCallback(evType C.int, x C.double, y C.double, keyCode C.int) {
	activeMu.RLock()
	tl := activeTimeline
	elapsedFn := activeElapsed
	activeMu.RUnlock()

	if tl == nil || elapsedFn == nil {
		return
	}

	ev := model.InteractionEvent{
		Timestamp: elapsedFn(),
	}
	switch int(evType) {
	case 1:
		ev.Type = model.InteractionMouseDown
		ev.Location = &model.Point{X: float64(x), Y: float64(y)}
	case 2:
		ev.Type = model.InteractionRightClick
		ev.Location = &model.Point{X: float64(x), Y: float64(y)}
	case 3:
		ev.Type = model.InteractionKeyPress
		ev.KeyCommand = fmt.Sprintf("key:%d", int(keyCode))
	default:
		return
	}
	tl.Record(ev)
}

type darwinTracker struct {
	mu      sync.Mutex
	running bool
}

// NewPlatformTracker creates the macOS CGEventTap-backed interaction tracker.
func NewPlatformTracker() Tracker {
	return &darwinTracker{}
}

func (d *darwinTracker) AccessibilityStatus() string {
	if bool(C.isAccessibilityTrusted()) {
		return "granted"
	}
	return "denied"
}

func (d *darwinTracker) Start(elapsedFn func() float64, timeline *Timeline) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.running {
		return nil
	}

	activeMu.Lock()
	activeTimeline = timeline
	activeElapsed = elapsedFn
	activeMu.Unlock()

	if !bool(C.isAccessibilityTrusted()) {
		// Graceful degradation: recording continues without OS-wide CGEventTap
		// until Accessibility permission is granted in macOS System Settings.
		return nil
	}

	d.running = true
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		C.startMacEventTap()
	}()
	return nil
}

func (d *darwinTracker) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.running {
		C.stopMacEventTap()
		d.running = false
	}
	activeMu.Lock()
	activeTimeline = nil
	activeElapsed = nil
	activeMu.Unlock()
}
