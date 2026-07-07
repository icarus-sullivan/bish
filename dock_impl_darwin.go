package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa

#import <Cocoa/Cocoa.h>
#import <objc/runtime.h>

extern void goOpenRecentDock(char* path);

static NSMenu* gDockMenu = nil;
static id gBishDockTarget = nil;

static NSMenu* bishDockMenuImpl(id self, SEL _cmd, NSApplication* app) {
    return gDockMenu;
}

@interface BishDockTarget : NSObject
@end
@implementation BishDockTarget
- (void)openProject:(NSMenuItem*)item {
    goOpenRecentDock((char*)[(NSString*)item.representedObject UTF8String]);
}
@end

void setBishDockMenuC(char** paths, char** names, int n) {
    NSMutableArray* pathArr = [NSMutableArray arrayWithCapacity:n];
    NSMutableArray* nameArr = [NSMutableArray arrayWithCapacity:n];
    for (int i = 0; i < n; i++) {
        [pathArr addObject:[NSString stringWithUTF8String:paths[i]]];
        [nameArr addObject:[NSString stringWithUTF8String:names[i]]];
    }
    dispatch_async(dispatch_get_main_queue(), ^{
        if (!gBishDockTarget) {
            gBishDockTarget = [[BishDockTarget alloc] init];
        }
        NSMenu* menu = [[NSMenu alloc] initWithTitle:@""];
        for (NSUInteger i = 0; i < (NSUInteger)pathArr.count; i++) {
            NSMenuItem* item = [[NSMenuItem alloc]
                initWithTitle:nameArr[i]
                action:@selector(openProject:)
                keyEquivalent:@""];
            item.target = gBishDockTarget;
            item.representedObject = pathArr[i];
            [menu addItem:item];
        }
        gDockMenu = menu;

        Class cls = object_getClass(NSApp.delegate);
        SEL sel = @selector(applicationDockMenu:);
        if (!class_respondsToSelector(cls, sel)) {
            class_addMethod(cls, sel, (IMP)bishDockMenuImpl, "@@:@");
        } else {
            method_setImplementation(class_getInstanceMethod(cls, sel), (IMP)bishDockMenuImpl);
        }
    });
}
*/
import "C"
