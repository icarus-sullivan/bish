package app

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

static int bishActivatePID(int pid) {
    NSRunningApplication* target = [NSRunningApplication runningApplicationWithProcessIdentifier:(pid_t)pid];
    if (!target) {
        return 0;
    }
    // options:0 (not NSApplicationActivateIgnoringOtherApps) — that flag is
    // deprecated as of macOS 14 and a no-op there anyway; plain activation
    // still raises the target app's window on all supported versions.
    return [target activateWithOptions:0] ? 1 : 0;
}
*/
import "C"

// activateWindow brings another bish window (a separate OS process, pid) to
// the front — used by OpenRecentProject to focus an already-open project
// instead of spawning a duplicate window for it. NSRunningApplication
// activation, unlike AppleScript's "tell application System Events" idiom,
// doesn't require the user to grant an Automation/Accessibility permission.
func activateWindow(pid int) bool {
	return C.bishActivatePID(C.int(pid)) != 0
}
