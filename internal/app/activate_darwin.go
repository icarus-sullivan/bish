package app

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

// Returns 1 if pid is a running bish window (same bundle, or same executable
// for unbundled dev builds) — guards against signaling a recycled pid that
// now belongs to an unrelated process, where SIGUSR2's default action would
// kill it.
static int bishIsBishPID(int pid) {
    NSRunningApplication* target = [NSRunningApplication runningApplicationWithProcessIdentifier:(pid_t)pid];
    if (!target) {
        return 0;
    }
    NSString* myID = [[NSBundle mainBundle] bundleIdentifier];
    if (myID && [target.bundleIdentifier isEqualToString:myID]) {
        return 1;
    }
    NSURL* myExe = [[NSBundle mainBundle] executableURL];
    return (myExe && [target.executableURL.path isEqualToString:myExe.path]) ? 1 : 0;
}

// Hands activation to pid before it is asked to raise itself. macOS 14 uses
// cooperative activation: the active app must yield first or the target's
// own activation request may be ignored. Synchronous so the yield is in
// place before the raise signal goes out.
static void bishYieldToPID(int pid) {
    NSRunningApplication* target = [NSRunningApplication runningApplicationWithProcessIdentifier:(pid_t)pid];
    if (!target) {
        return;
    }
    void (^yield)(void) = ^{
        if (@available(macOS 14.0, *)) {
            [NSApp yieldActivationToApplication:target];
        }
        [target activateWithOptions:NSApplicationActivateAllWindows];
    };
    if ([NSThread isMainThread]) {
        yield();
    } else {
        dispatch_sync(dispatch_get_main_queue(), yield);
    }
}

// Raises this process's windows: un-minimize, bring to front, take focus.
// Activation alone doesn't restore a minimized window, and child windows
// (accessory policy, no Dock icon) have no Dock tile the user could click.
static void bishRaiseSelf(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"
        [NSApp activateIgnoringOtherApps:YES];
#pragma clang diagnostic pop
        for (NSWindow* w in [NSApp windows]) {
            if (!w.canBecomeMainWindow) {
                continue;
            }
            if (w.isMiniaturized) {
                [w deminiaturize:nil];
            }
            [w makeKeyAndOrderFront:nil];
        }
    });
}
*/
import "C"

import (
	"os"
	"os/signal"
	"syscall"
)

// raiseSignal asks another bish window (a separate process) to raise itself
// — see installRaiseHandler. SIGUSR1 is already taken by Dock-representation
// promotion (promote_signal_unix.go in package main).
const raiseSignal = syscall.SIGUSR2

// activateWindow brings another bish window (a separate OS process, pid) to
// the front — used to focus an already-open project instead of spawning a
// duplicate window for it. This process yields activation to the target and
// signals it to raise its own window from the inside, which (unlike
// NSRunningApplication activation alone) also un-minimizes it. Neither needs
// an Automation/Accessibility permission.
func activateWindow(pid int) bool {
	if C.bishIsBishPID(C.int(pid)) == 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	C.bishYieldToPID(C.int(pid))
	return proc.Signal(raiseSignal) == nil
}

// installRaiseHandler raises this window when another bish process sends
// raiseSignal.
func (a *App) installRaiseHandler() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, raiseSignal)
	go func() {
		for range ch {
			C.bishRaiseSelf()
		}
	}()
}
