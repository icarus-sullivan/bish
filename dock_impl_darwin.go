package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa

#import <Cocoa/Cocoa.h>
#import <objc/runtime.h>

extern void goNewWindowDock(void);
extern void goNewFileDock(void);

static NSMenu* gDockMenu = nil;
static id gBishDockTarget = nil;

static NSMenu* bishDockMenuImpl(id self, SEL _cmd, NSApplication* app) {
    return gDockMenu;
}

@interface BishDockTarget : NSObject
@end
@implementation BishDockTarget
- (void)newWindow:(NSMenuItem*)item {
    goNewWindowDock();
}
- (void)newFile:(NSMenuItem*)item {
    goNewFileDock();
}
@end

void setBishDockMenuC(char** projPaths, char** projNames, int projN,
                       char** filePaths, char** fileNames, int fileN) {
    NSMutableArray* pPathArr = [NSMutableArray arrayWithCapacity:projN];
    for (int i = 0; i < projN; i++) {
        [pPathArr addObject:[NSString stringWithUTF8String:projPaths[i]]];
    }
    NSMutableArray* fPathArr = [NSMutableArray arrayWithCapacity:fileN];
    for (int i = 0; i < fileN; i++) {
        [fPathArr addObject:[NSString stringWithUTF8String:filePaths[i]]];
    }
    dispatch_async(dispatch_get_main_queue(), ^{
        if (!gBishDockTarget) {
            gBishDockTarget = [[BishDockTarget alloc] init];
        }
        NSMenu* menu = [[NSMenu alloc] initWithTitle:@""];
        NSMenuItem* newWindow = [[NSMenuItem alloc]
            initWithTitle:@"New Window"
            action:@selector(newWindow:)
            keyEquivalent:@""];
        newWindow.target = gBishDockTarget;
        [menu addItem:newWindow];
        NSMenuItem* newFile = [[NSMenuItem alloc]
            initWithTitle:@"New File"
            action:@selector(newFile:)
            keyEquivalent:@""];
        newFile.target = gBishDockTarget;
        [menu addItem:newFile];
        gDockMenu = menu;

        // Match VS Code and native document apps: recents belong to macOS,
        // which adds its standard Recent section to the Dock menu. Clicking
        // one delivers application:openFile: → mac.OnFileOpen (main.go).
        // Add oldest first so the most recently used item ends up first.
        NSDocumentController* docs = [NSDocumentController sharedDocumentController];
        [docs clearRecentDocuments:nil];
        for (NSInteger i = (NSInteger)fPathArr.count - 1; i >= 0; i--) {
            [docs noteNewRecentDocumentURL:[NSURL fileURLWithPath:fPathArr[(NSUInteger)i]]];
        }
        for (NSInteger i = (NSInteger)pPathArr.count - 1; i >= 0; i--) {
            [docs noteNewRecentDocumentURL:[NSURL fileURLWithPath:pPathArr[(NSUInteger)i]]];
        }

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
