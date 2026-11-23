//go:build darwin

#import <Cocoa/Cocoa.h>
#include <math.h>
#include <stdbool.h>

extern void goAppWillTerminate(void);
extern void goWindowDidResize(int width, bool inLiveResize);

static BOOL gCleanupRegistered = NO;
static BOOL gApplyingProgrammaticFrame = NO;

void setupMacMainMenu(void) {
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

void setMacAppIcon(const void *pngBytes, int length) {
    if (pngBytes == NULL || length <= 0) return;
    NSData *data = [NSData dataWithBytes:pngBytes length:(NSUInteger)length];
    dispatch_async(dispatch_get_main_queue(), ^{
        NSImage *img = [[NSImage alloc] initWithData:data];
        if (img != nil) {
            [NSApp setApplicationIconImage:img];
        }
    });
}

void configureMacWindow(void *winPtr, bool floatingCompact, int minWidth, int targetWidth, int targetHeight) {
    if (winPtr == NULL) return;
    NSWindow *win = (__bridge NSWindow *)winPtr;
    dispatch_async(dispatch_get_main_queue(), ^{
        // Self-exclusion guarantee: prevent macOS screen capture APIs from recording our own window.
        [win setSharingType:NSWindowSharingNone];
        [win setAppearance:[NSAppearance appearanceNamed:NSAppearanceNameDarkAqua]];

        // Borderless-look floating HUD: hide titlebar & traffic lights while keeping
        // full keyboard focus (canBecomeKeyWindow) + native rounded corners & shadow.
        [win setStyleMask:([win styleMask] | NSWindowStyleMaskFullSizeContentView)];
        [win setTitleVisibility:NSWindowTitleHidden];
        [win setTitlebarAppearsTransparent:YES];
        [[win standardWindowButton:NSWindowCloseButton] setHidden:YES];
        [[win standardWindowButton:NSWindowMiniaturizeButton] setHidden:YES];
        [[win standardWindowButton:NSWindowZoomButton] setHidden:YES];
        [win setMovableByWindowBackground:YES];
        [win setBackgroundColor:[NSColor colorWithRed:0.02 green:0.02 blue:0.03 alpha:1.0]];

        if (targetWidth > 0 && targetHeight > 0) {
            CGFloat minW = (CGFloat)(minWidth > 0 ? minWidth : 760);
            CGFloat targetW = (CGFloat)targetWidth;
            if (targetW < minW) {
                targetW = minW;
            }
            CGFloat targetH = (CGFloat)targetHeight;

            gApplyingProgrammaticFrame = YES;
            // Lock vertical resizing by fixing minSize.height == maxSize.height == targetH
            // while allowing smooth horizontal resizing from minW upward.
            [win setMinSize:NSMakeSize(minW, targetH)];
            [win setMaxSize:NSMakeSize(10000.0, targetH)];

            NSRect f = [win frame];
            CGFloat deltaH = targetH - f.size.height;
            f.origin.y -= deltaH; // keep top edge anchored when resizing
            f.size.width = targetW;
            f.size.height = targetH;
            [win setFrame:f display:YES animate:NO];
            gApplyingProgrammaticFrame = NO;
        }

        if (floatingCompact) {
            [win setLevel:NSFloatingWindowLevel];
            [win setCollectionBehavior:NSWindowCollectionBehaviorCanJoinAllSpaces | NSWindowCollectionBehaviorFullScreenAuxiliary];
        } else {
            [win setLevel:NSNormalWindowLevel];
        }

        if (!gCleanupRegistered) {
            gCleanupRegistered = YES;

            // Track horizontal user resizes so width persists across restarts and drawer toggles.
            [[NSNotificationCenter defaultCenter]
                addObserverForName:NSWindowDidResizeNotification
                            object:win
                             queue:[NSOperationQueue mainQueue]
                        usingBlock:^(NSNotification * _Nonnull note) {
                            if (gApplyingProgrammaticFrame) return;
                            int w = (int)llround([win frame].size.width);
                            goWindowDidResize(w, [win inLiveResize] ? true : false);
                        }];

            [[NSNotificationCenter defaultCenter]
                addObserverForName:NSWindowDidEndLiveResizeNotification
                            object:win
                             queue:[NSOperationQueue mainQueue]
                        usingBlock:^(NSNotification * _Nonnull note) {
                            if (gApplyingProgrammaticFrame) return;
                            int w = (int)llround([win frame].size.width);
                            goWindowDidResize(w, false);
                        }];

            // Closing the main Control Plane window (red X or Cmd+W) terminates the entire app
            // cleanly so the detached Capture Box never keeps XNK Iterate lingering in the background.
            [[NSNotificationCenter defaultCenter]
                addObserverForName:NSWindowWillCloseNotification
                            object:win
                             queue:[NSOperationQueue mainQueue]
                        usingBlock:^(NSNotification * _Nonnull note) {
                            goAppWillTerminate();
                            [NSApp terminate:nil];
                        }];

            // Also run cleanup on Cmd+Q before process exit so all child helpers stop cleanly.
            [[NSNotificationCenter defaultCenter]
                addObserverForName:NSApplicationWillTerminateNotification
                            object:NSApp
                             queue:[NSOperationQueue mainQueue]
                        usingBlock:^(NSNotification * _Nonnull note) {
                            goAppWillTerminate();
                        }];
        }
    });
}

void startMacWindowDrag(void *winPtr) {
    if (winPtr == NULL) return;
    NSWindow *win = (__bridge NSWindow *)winPtr;
    void (^doDrag)(void) = ^{
        NSEvent *evt = [NSApp currentEvent];
        if (evt != nil) {
            [win performWindowDragWithEvent:evt];
        }
    };
    if ([NSThread isMainThread]) {
        doDrag();
    } else {
        dispatch_async(dispatch_get_main_queue(), doDrag);
    }
}
