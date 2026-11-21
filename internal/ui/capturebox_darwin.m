//go:build darwin

#import <Cocoa/Cocoa.h>
#include <stdbool.h>

extern void goCaptureBoxMoved(int x, int y, int w, int h, int displayIdx);

typedef NS_ENUM(NSInteger, XNKDragMode) {
    XNKDragNone = 0,
    XNKDragMove,
    XNKDragTL,
    XNKDragTR,
    XNKDragBL,
    XNKDragBR,
    XNKDragT,
    XNKDragB,
    XNKDragL,
    XNKDragR
};

@interface XNKCaptureOverlayView : NSView
@property (nonatomic, assign) BOOL locked;
@property (nonatomic, assign) XNKDragMode dragMode;
@property (nonatomic, assign) NSPoint dragStartScreen;
@property (nonatomic, assign) NSRect dragStartFrame;
@end

static int xnkPrimaryScreenHeight(void) {
    NSArray<NSScreen *> *screens = [NSScreen screens];
    if (screens.count == 0) return 1080;
    return (int)screens[0].frame.size.height;
}

static int xnkDisplayIndexForRect(NSRect frame) {
    NSPoint center = NSMakePoint(NSMidX(frame), NSMidY(frame));
    NSArray<NSScreen *> *screens = [NSScreen screens];
    for (NSUInteger i = 0; i < screens.count; i++) {
        if (NSPointInRect(center, screens[i].frame)) {
            return (int)i;
        }
    }
    return 0;
}

static void xnkNotifyGo(NSRect cocoaFrame) {
    int primaryH = xnkPrimaryScreenHeight();
    int topLeftX = (int)cocoaFrame.origin.x;
    int topLeftY = primaryH - (int)(cocoaFrame.origin.y + cocoaFrame.size.height);
    int w = (int)cocoaFrame.size.width;
    int h = (int)cocoaFrame.size.height;
    int dispIdx = xnkDisplayIndexForRect(cocoaFrame);
    goCaptureBoxMoved(topLeftX, topLeftY, w, h, dispIdx);
}

@implementation XNKCaptureOverlayView

- (BOOL)isOpaque { return NO; }

- (void)drawRect:(NSRect)dirtyRect {
    [[NSColor clearColor] setFill];
    NSRectFill(dirtyRect);

    NSRect b = self.bounds;
    NSRect frameRect = NSInsetRect(b, 6.0, 6.0);

    NSColor *strokeColor = self.locked
        ? [NSColor colorWithRed:0.957 green:0.247 blue:0.369 alpha:0.95] // XNK Crimson (#f43f5e)
        : [NSColor colorWithRed:0.220 green:0.741 blue:0.973 alpha:0.98]; // XNK Icy Cyan (#38bdf8)

    NSBezierPath *border = [NSBezierPath bezierPathWithRoundedRect:frameRect xRadius:6.0 yRadius:6.0];
    [border setLineWidth:self.locked ? 2.0 : 2.5];
    if (self.locked) {
        CGFloat pattern[2] = {6.0, 4.0};
        [border setLineDash:pattern count:2 phase:0.0];
    }
    [strokeColor setStroke];
    [border stroke];

    NSRect badgeRect = NSMakeRect(16, b.size.height - 34, self.locked ? 210 : 230, 22);
    NSBezierPath *badgeBg = [NSBezierPath bezierPathWithRoundedRect:badgeRect xRadius:5.0 yRadius:5.0];
    [[NSColor colorWithRed:0.04 green:0.04 blue:0.06 alpha:0.88] setFill];
    [badgeBg fill];
    [strokeColor setStroke];
    [badgeBg setLineWidth:1.0];
    [badgeBg stroke];

    NSString *label = self.locked
        ? [NSString stringWithFormat:@"● REC LOCKED  %d × %d", (int)b.size.width, (int)b.size.height]
        : [NSString stringWithFormat:@"CAPTURE REGION  %d × %d", (int)b.size.width, (int)b.size.height];

    NSDictionary *attrs = @{
        NSFontAttributeName: [NSFont monospacedSystemFontOfSize:10.5 weight:NSFontWeightSemibold],
        NSForegroundColorAttributeName: strokeColor
    };
    [label drawAtPoint:NSMakePoint(badgeRect.origin.x + 10, badgeRect.origin.y + 4) withAttributes:attrs];

    if (!self.locked) {
        NSPoint handles[8] = {
            NSMakePoint(NSMinX(frameRect), NSMaxY(frameRect)),
            NSMakePoint(NSMaxX(frameRect), NSMaxY(frameRect)),
            NSMakePoint(NSMinX(frameRect), NSMinY(frameRect)),
            NSMakePoint(NSMaxX(frameRect), NSMinY(frameRect)),
            NSMakePoint(NSMidX(frameRect), NSMaxY(frameRect)),
            NSMakePoint(NSMidX(frameRect), NSMinY(frameRect)),
            NSMakePoint(NSMinX(frameRect), NSMidY(frameRect)),
            NSMakePoint(NSMaxX(frameRect), NSMidY(frameRect)),
        };
        for (int i = 0; i < 8; i++) {
            NSRect hr = NSMakeRect(handles[i].x - 5, handles[i].y - 5, 10, 10);
            NSBezierPath *hp = [NSBezierPath bezierPathWithRoundedRect:hr xRadius:2.0 yRadius:2.0];
            [[NSColor blackColor] setFill];
            [hp fill];
            [strokeColor setStroke];
            [hp setLineWidth:1.8];
            [hp stroke];
        }
    }
}

- (XNKDragMode)hitTestMode:(NSPoint)pt {
    NSRect b = self.bounds;
    CGFloat edge = 18.0;
    BOOL left = pt.x < edge;
    BOOL right = pt.x > b.size.width - edge;
    BOOL bottom = pt.y < edge;
    BOOL top = pt.y > b.size.height - edge;

    if (top && left) return XNKDragTL;
    if (top && right) return XNKDragTR;
    if (bottom && left) return XNKDragBL;
    if (bottom && right) return XNKDragBR;
    if (top) return XNKDragT;
    if (bottom) return XNKDragB;
    if (left) return XNKDragL;
    if (right) return XNKDragR;
    return XNKDragMove;
}

- (void)mouseDown:(NSEvent *)event {
    if (self.locked) return;
    NSPoint local = [self convertPoint:event.locationInWindow fromView:nil];
    self.dragMode = [self hitTestMode:local];
    self.dragStartScreen = [NSEvent mouseLocation];
    self.dragStartFrame = self.window.frame;
}

- (void)mouseDragged:(NSEvent *)event {
    if (self.locked || self.dragMode == XNKDragNone) return;
    NSPoint now = [NSEvent mouseLocation];
    CGFloat dx = now.x - self.dragStartScreen.x;
    CGFloat dy = now.y - self.dragStartScreen.y;
    NSRect f = self.dragStartFrame;
    CGFloat minW = 240.0, minH = 160.0;

    switch (self.dragMode) {
        case XNKDragMove:
            f.origin.x += dx;
            f.origin.y += dy;
            break;
        case XNKDragR:
            f.size.width = MAX(minW, f.size.width + dx);
            break;
        case XNKDragL: {
            CGFloat newW = MAX(minW, f.size.width - dx);
            f.origin.x += (f.size.width - newW);
            f.size.width = newW;
            break;
        }
        case XNKDragT:
            f.size.height = MAX(minH, f.size.height + dy);
            break;
        case XNKDragB: {
            CGFloat newH = MAX(minH, f.size.height - dy);
            f.origin.y += (f.size.height - newH);
            f.size.height = newH;
            break;
        }
        case XNKDragTR:
            f.size.width = MAX(minW, f.size.width + dx);
            f.size.height = MAX(minH, f.size.height + dy);
            break;
        case XNKDragTL: {
            CGFloat newW = MAX(minW, f.size.width - dx);
            f.origin.x += (f.size.width - newW);
            f.size.width = newW;
            f.size.height = MAX(minH, f.size.height + dy);
            break;
        }
        case XNKDragBR: {
            f.size.width = MAX(minW, f.size.width + dx);
            CGFloat newH = MAX(minH, f.size.height - dy);
            f.origin.y += (f.size.height - newH);
            f.size.height = newH;
            break;
        }
        case XNKDragBL: {
            CGFloat newW = MAX(minW, f.size.width - dx);
            f.origin.x += (f.size.width - newW);
            f.size.width = newW;
            CGFloat newH = MAX(minH, f.size.height - dy);
            f.origin.y += (f.size.height - newH);
            f.size.height = newH;
            break;
        }
        default:
            break;
    }

    [self.window setFrame:f display:YES];
    [self setNeedsDisplay:YES];
    xnkNotifyGo(f);
}

- (void)mouseUp:(NSEvent *)event {
    self.dragMode = XNKDragNone;
}

@end

static NSWindow *gCaptureBoxWin = NULL;
static XNKCaptureOverlayView *gCaptureBoxView = NULL;

void xnkShowCaptureBox(int x, int y, int w, int h, bool locked) {
    dispatch_async(dispatch_get_main_queue(), ^{
        int primaryH = xnkPrimaryScreenHeight();
        NSRect frame = NSMakeRect(x, primaryH - (y + h), MAX(240, w), MAX(160, h));

        if (gCaptureBoxWin == NULL) {
            gCaptureBoxWin = [[NSWindow alloc] initWithContentRect:frame
                                                         styleMask:NSWindowStyleMaskBorderless
                                                           backing:NSBackingStoreBuffered
                                                             defer:NO];
            [gCaptureBoxWin setOpaque:NO];
            [gCaptureBoxWin setBackgroundColor:[NSColor clearColor]];
            [gCaptureBoxWin setHasShadow:NO];
            [gCaptureBoxWin setLevel:NSFloatingWindowLevel];
            [gCaptureBoxWin setSharingType:NSWindowSharingNone];
            [gCaptureBoxWin setCollectionBehavior:NSWindowCollectionBehaviorCanJoinAllSpaces | NSWindowCollectionBehaviorFullScreenAuxiliary];

            gCaptureBoxView = [[XNKCaptureOverlayView alloc] initWithFrame:NSMakeRect(0, 0, frame.size.width, frame.size.height)];
            [gCaptureBoxView setAutoresizingMask:NSViewWidthSizable | NSViewHeightSizable];
            [gCaptureBoxWin setContentView:gCaptureBoxView];
        } else {
            [gCaptureBoxWin setFrame:frame display:YES];
        }

        gCaptureBoxView.locked = locked ? YES : NO;
        [gCaptureBoxWin setIgnoresMouseEvents:locked ? YES : NO];
        [gCaptureBoxView setNeedsDisplay:YES];
        [gCaptureBoxWin orderFrontRegardless];
    });
}

void xnkHideCaptureBox(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (gCaptureBoxWin != NULL) {
            [gCaptureBoxWin orderOut:nil];
        }
    });
}

void xnkSetCaptureBoxLocked(bool locked) {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (gCaptureBoxWin != NULL && gCaptureBoxView != NULL) {
            gCaptureBoxView.locked = locked ? YES : NO;
            [gCaptureBoxWin setIgnoresMouseEvents:locked ? YES : NO];
            [gCaptureBoxView setNeedsDisplay:YES];
        }
    });
}

void xnkDestroyCaptureBox(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (gCaptureBoxWin != NULL) {
            [gCaptureBoxWin close];
            gCaptureBoxWin = NULL;
            gCaptureBoxView = NULL;
        }
    });
}
