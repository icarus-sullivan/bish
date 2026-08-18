package main

/*
#import <Cocoa/Cocoa.h>

// The reverse of hidedock_impl_darwin.go: a window being promoted takes over
// Dock-icon/menu-bar representation from a bish window that's about to exit
// (see App.Shutdown). Queued to the main queue like hideDockIcon, since this
// runs from a signal handler on an arbitrary goroutine, not the AppKit
// thread.
void bishPromoteToRegularC(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        [NSApp setActivationPolicy:NSApplicationActivationPolicyRegular];
        [NSApp activateIgnoringOtherApps:YES];
    });
}
*/
import "C"
