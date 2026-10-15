package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa

#import <Cocoa/Cocoa.h>

extern void goLaunchSettled(void);

void bishObserveLaunchC(void) {
    [[NSNotificationCenter defaultCenter]
        addObserverForName:NSApplicationDidFinishLaunchingNotification
        object:nil
        queue:nil
        usingBlock:^(NSNotification* note) {
            goLaunchSettled();
        }];
}
*/
import "C"
