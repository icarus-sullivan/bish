package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa

#import <Cocoa/Cocoa.h>
#import <objc/runtime.h>

extern void goMarkQuitRequested(void);

static IMP gOrigApplicationShouldTerminate = NULL;

// Swizzles AppDelegate's applicationShouldTerminate: so we can tell "Quit"
// (Cmd+Q / app menu) apart from closing a single window — both otherwise
// reach Wails' shutdown path with no distinguishing signal at the Go level.
// Chains to the original implementation so Wails' own handling is untouched.
static NSApplicationTerminateReply bishApplicationShouldTerminate(id self, SEL _cmd, NSApplication *app) {
    goMarkQuitRequested();
    if (gOrigApplicationShouldTerminate) {
        NSApplicationTerminateReply (*orig)(id, SEL, NSApplication *) = (void *)gOrigApplicationShouldTerminate;
        return orig(self, _cmd, app);
    }
    return NSTerminateNow;
}

void installQuitInterceptC(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        Class cls = object_getClass(NSApp.delegate);
        SEL sel = @selector(applicationShouldTerminate:);
        Method m = class_getInstanceMethod(cls, sel);
        if (m) {
            gOrigApplicationShouldTerminate = method_getImplementation(m);
            method_setImplementation(m, (IMP)bishApplicationShouldTerminate);
        }
    });
}
*/
import "C"
