//go:build darwin

#import <Cocoa/Cocoa.h>
#include <stdlib.h>
#include <string.h>

char *xnkPickFolder(const char *initialDir) {
    __block char *result = NULL;
    void (^showPanel)(void) = ^{
        NSOpenPanel *panel = [NSOpenPanel openPanel];
        [panel setCanChooseFiles:NO];
        [panel setCanChooseDirectories:YES];
        [panel setAllowsMultipleSelection:NO];
        [panel setCanCreateDirectories:YES];
        [panel setTitle:@"Select Antigravity Workspace Folder"];
        [panel setPrompt:@"Select Folder"];

        if (initialDir != NULL && strlen(initialDir) > 0) {
            NSString *dirStr = [NSString stringWithUTF8String:initialDir];
            NSURL *url = [NSURL fileURLWithPath:dirStr isDirectory:YES];
            [panel setDirectoryURL:url];
        }

        if ([panel runModal] == NSModalResponseOK) {
            NSURL *selected = [[panel URLs] firstObject];
            if (selected != nil && [selected path] != nil) {
                const char *utf8 = [[selected path] UTF8String];
                if (utf8 != NULL) {
                    result = strdup(utf8);
                }
            }
        }
    };

    if ([NSThread isMainThread]) {
        showPanel();
    } else {
        dispatch_sync(dispatch_get_main_queue(), showPanel);
    }
    return result;
}
