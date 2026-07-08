package main

/*
#import <Cocoa/Cocoa.h>

// Child windows drop to the accessory activation policy so only the primary
// instance shows a Dock icon. Queued to the main queue: it runs once the
// NSApp run loop starts, after wails has created the application.
void bishHideDockIconC(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        [NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
        [NSApp activateIgnoringOtherApps:YES];
    });
}
*/
import "C"
