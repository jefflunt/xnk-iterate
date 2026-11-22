//go:build darwin

#import <Carbon/Carbon.h>
#import <Cocoa/Cocoa.h>

extern void goGlobalHotkeyPressed(int hotkeyID);

static EventHotKeyRef gSendHotKeyRef = NULL;
static EventHotKeyRef gDiscardHotKeyRef = NULL;
static EventHandlerRef gHotKeyHandlerRef = NULL;

static uint32_t gSendKeyCode = kVK_Return;
static uint32_t gSendMods = optionKey;
static uint32_t gDiscardKeyCode = kVK_Delete;
static uint32_t gDiscardMods = optionKey;
static int gHotkeysSuspended = 0;

static uint32_t xnkToCarbonMods(uint32_t mask) {
    uint32_t mods = 0;
    if (mask & (1u << 0)) mods |= cmdKey;
    if (mask & (1u << 1)) mods |= shiftKey;
    if (mask & (1u << 2)) mods |= optionKey;
    if (mask & (1u << 3)) mods |= controlKey;
    return mods;
}

static OSStatus xnkHotKeyHandler(EventHandlerCallRef nextHandler, EventRef theEvent, void *userData) {
    EventHotKeyID hkID;
    OSStatus err = GetEventParameter(
        theEvent,
        kEventParamDirectObject,
        typeEventHotKeyID,
        NULL,
        sizeof(hkID),
        NULL,
        &hkID
    );
    if (err == noErr) {
        goGlobalHotkeyPressed((int)hkID.id);
    }
    return noErr;
}

static void xnkUnregisterRefsOnly(void) {
    if (gSendHotKeyRef != NULL) {
        UnregisterEventHotKey(gSendHotKeyRef);
        gSendHotKeyRef = NULL;
    }
    if (gDiscardHotKeyRef != NULL) {
        UnregisterEventHotKey(gDiscardHotKeyRef);
        gDiscardHotKeyRef = NULL;
    }
}

static void xnkRegisterCurrentRefs(void) {
    if (gHotkeysSuspended) return;

    if (gHotKeyHandlerRef == NULL) {
        EventTypeSpec eventType;
        eventType.eventClass = kEventClassKeyboard;
        eventType.eventKind = kEventHotKeyPressed;
        InstallApplicationEventHandler(&xnkHotKeyHandler, 1, &eventType, NULL, &gHotKeyHandlerRef);
    }

    xnkUnregisterRefsOnly();

    EventHotKeyID sendID;
    sendID.signature = 'XNKI';
    sendID.id = 1;
    RegisterEventHotKey(gSendKeyCode, gSendMods, sendID, GetApplicationEventTarget(), 0, &gSendHotKeyRef);

    EventHotKeyID discardID;
    discardID.signature = 'XNKI';
    discardID.id = 2;
    RegisterEventHotKey(gDiscardKeyCode, gDiscardMods, discardID, GetApplicationEventTarget(), 0, &gDiscardHotKeyRef);
}

void xnkUpdateGlobalHotkeys(uint32_t sendKeyCode, uint32_t sendModMask, uint32_t discardKeyCode, uint32_t discardModMask) {
    uint32_t sMods = xnkToCarbonMods(sendModMask);
    uint32_t dMods = xnkToCarbonMods(discardModMask);
    dispatch_async(dispatch_get_main_queue(), ^{
        gSendKeyCode = sendKeyCode;
        gSendMods = sMods;
        gDiscardKeyCode = discardKeyCode;
        gDiscardMods = dMods;
        xnkRegisterCurrentRefs();
    });
}

void xnkSuspendGlobalHotkeys(int suspended) {
    dispatch_async(dispatch_get_main_queue(), ^{
        gHotkeysSuspended = suspended ? 1 : 0;
        if (gHotkeysSuspended) {
            xnkUnregisterRefsOnly();
        } else {
            xnkRegisterCurrentRefs();
        }
    });
}

void xnkUnregisterGlobalHotkeys(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        gHotkeysSuspended = 1;
        xnkUnregisterRefsOnly();
        if (gHotKeyHandlerRef != NULL) {
            RemoveEventHandler(gHotKeyHandlerRef);
            gHotKeyHandlerRef = NULL;
        }
    });
}
