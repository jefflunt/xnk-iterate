//go:build darwin

package ui

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>
#include <stdbool.h>

static void setupMacMainMenu(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        NSMenu *menubar = [[NSMenu alloc] init];

        // App Menu (Cmd+H, Cmd+Q)
        NSMenuItem *appMenuItem = [[NSMenuItem alloc] init];
        [menubar addItem:appMenuItem];
        NSMenu *appMenu = [[NSMenu alloc] initWithTitle:@"XNK Iterate"];
        [appMenu addItemWithTitle:@"Hide XNK Iterate" action:@selector(hide:) keyEquivalent:@"h"];
        [appMenu addItem:[NSMenuItem separatorItem]];
        [appMenu addItemWithTitle:@"Quit XNK Iterate" action:@selector(terminate:) keyEquivalent:@"q"];
        [appMenuItem setSubmenu:appMenu];

        // Edit Menu (Cmd+Z, Cmd+X, Cmd+C, Cmd+V, Cmd+A)
        NSMenuItem *editMenuItem = [[NSMenuItem alloc] init];
        [menubar addItem:editMenuItem];
        NSMenu *editMenu = [[NSMenu alloc] initWithTitle:@"Edit"];
        [editMenu addItemWithTitle:@"Undo" action:@selector(undo:) keyEquivalent:@"z"];
        [editMenu addItemWithTitle:@"Redo" action:@selector(redo:) keyEquivalent:@"Z"];
        [editMenu addItem:[NSMenuItem separatorItem]];
        [editMenu addItemWithTitle:@"Cut" action:@selector(cut:) keyEquivalent:@"x"];
        [editMenu addItemWithTitle:@"Copy" action:@selector(copy:) keyEquivalent:@"c"];
        [editMenu addItemWithTitle:@"Paste" action:@selector(paste:) keyEquivalent:@"v"];
        [editMenu addItemWithTitle:@"Select All" action:@selector(selectAll:) keyEquivalent:@"a"];
        [editMenuItem setSubmenu:editMenu];

        // Window Menu (Cmd+W, Cmd+M)
        NSMenuItem *winMenuItem = [[NSMenuItem alloc] init];
        [menubar addItem:winMenuItem];
        NSMenu *winMenu = [[NSMenu alloc] initWithTitle:@"Window"];
        [winMenu addItemWithTitle:@"Minimize" action:@selector(performMiniaturize:) keyEquivalent:@"m"];
        [winMenu addItemWithTitle:@"Close" action:@selector(performClose:) keyEquivalent:@"w"];
        [winMenuItem setSubmenu:winMenu];

        [NSApp setMainMenu:menubar];
    });
}

static void setMacAppIcon(const void *pngBytes, int length) {
    if (pngBytes == NULL || length <= 0) return;
    NSData *data = [NSData dataWithBytes:pngBytes length:(NSUInteger)length];
    dispatch_async(dispatch_get_main_queue(), ^{
        NSImage *img = [[NSImage alloc] initWithData:data];
        if (img != nil) {
            [NSApp setApplicationIconImage:img];
        }
    });
}

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
import (
	_ "embed"
	"sync"
	"unsafe"
)

//go:embed assets/icon.png
var embeddedAppIconPNG []byte

var appIconOnce sync.Once

func configureNativeWindow(windowPtr unsafe.Pointer, floatingCompact bool) {
	appIconOnce.Do(func() {
		C.setupMacMainMenu()
		if len(embeddedAppIconPNG) > 0 {
			C.setMacAppIcon(unsafe.Pointer(&embeddedAppIconPNG[0]), C.int(len(embeddedAppIconPNG)))
		}
	})
	if windowPtr == nil {
		return
	}
	C.configureMacWindow(windowPtr, C.bool(floatingCompact))
}
