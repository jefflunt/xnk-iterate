//go:build darwin

package ui

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>
#include <stdbool.h>

static void configureMacWindow(void *winPtr, bool floatingCompact) {
    if (winPtr == NULL) return;
    NSWindow *win = (__bridge NSWindow *)winPtr;
    dispatch_async(dispatch_get_main_queue(), ^{
        // Self-exclusion guarantee: prevent macOS screen capture APIs from recording our own HUD/Studio window.
        [win setSharingType:NSWindowSharingNone];
        [win setAppearance:[NSAppearance appearanceNamed:NSAppearanceNameDarkAqua]];
        [win setTitlebarAppearsTransparent:YES];
        [win setBackgroundColor:[NSColor colorWithRed:0.02 green:0.02 blue:0.03 alpha:1.0]];

        if (floatingCompact) {
            [win setLevel:NSFloatingWindowLevel];
            [win setCollectionBehavior:NSWindowCollectionBehaviorCanJoinAllSpaces | NSWindowCollectionBehaviorFullScreenAuxiliary];
        } else {
            [win setLevel:NSNormalWindowLevel];
        }
    });
}
*/
import "C"
import "unsafe"

func configureNativeWindow(windowPtr unsafe.Pointer, floatingCompact bool) {
	if windowPtr == nil {
		return
	}
	C.configureMacWindow(windowPtr, C.bool(floatingCompact))
}
