package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa

#import <Cocoa/Cocoa.h>
#import <objc/runtime.h>

extern void goOpenRecentDock(char* path);
extern void goOpenRecentFileDock(char* path);

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
- (void)openFile:(NSMenuItem*)item {
    goOpenRecentFileDock((char*)[(NSString*)item.representedObject UTF8String]);
}
@end

void setBishDockMenuC(char** projPaths, char** projNames, int projN,
                       char** filePaths, char** fileNames, int fileN) {
    NSMutableArray* pPathArr = [NSMutableArray arrayWithCapacity:projN];
    NSMutableArray* pNameArr = [NSMutableArray arrayWithCapacity:projN];
    for (int i = 0; i < projN; i++) {
        [pPathArr addObject:[NSString stringWithUTF8String:projPaths[i]]];
        [pNameArr addObject:[NSString stringWithUTF8String:projNames[i]]];
    }
    NSMutableArray* fPathArr = [NSMutableArray arrayWithCapacity:fileN];
    NSMutableArray* fNameArr = [NSMutableArray arrayWithCapacity:fileN];
    for (int i = 0; i < fileN; i++) {
        [fPathArr addObject:[NSString stringWithUTF8String:filePaths[i]]];
        [fNameArr addObject:[NSString stringWithUTF8String:fileNames[i]]];
    }
    dispatch_async(dispatch_get_main_queue(), ^{
        if (!gBishDockTarget) {
            gBishDockTarget = [[BishDockTarget alloc] init];
        }
        NSMenu* menu = [[NSMenu alloc] initWithTitle:@""];
        for (NSUInteger i = 0; i < (NSUInteger)pPathArr.count; i++) {
            NSMenuItem* item = [[NSMenuItem alloc]
                initWithTitle:pNameArr[i]
                action:@selector(openProject:)
                keyEquivalent:@""];
            item.target = gBishDockTarget;
            item.representedObject = pPathArr[i];
            [menu addItem:item];
        }
        if (pPathArr.count > 0 && fPathArr.count > 0) {
            [menu addItem:[NSMenuItem separatorItem]];
        }
        for (NSUInteger i = 0; i < (NSUInteger)fPathArr.count; i++) {
            NSMenuItem* item = [[NSMenuItem alloc]
                initWithTitle:fNameArr[i]
                action:@selector(openFile:)
                keyEquivalent:@""];
            item.target = gBishDockTarget;
            item.representedObject = fPathArr[i];
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

// noteBishRecentDocumentsC feeds NSDocumentController's recent-documents
// store, the OS-persisted list (separate from gDockMenu above) that macOS
// reads to build the Dock icon's context menu when bish isn't running —
// gDockMenu only exists in-process, so without this the right-click menu on
// a quit bish is empty. Called in reverse order so paths[0] (most recent)
// ends up on top.
void noteBishRecentDocumentsC(char** paths, int n) {
    NSMutableArray* pathArr = [NSMutableArray arrayWithCapacity:n];
    for (int i = 0; i < n; i++) {
        [pathArr addObject:[NSString stringWithUTF8String:paths[i]]];
    }
    dispatch_async(dispatch_get_main_queue(), ^{
        NSDocumentController* dc = [NSDocumentController sharedDocumentController];
        for (NSInteger i = (NSInteger)pathArr.count - 1; i >= 0; i--) {
            NSURL* url = [NSURL fileURLWithPath:pathArr[i]];
            [dc noteNewRecentDocumentURL:url];
        }
    });
}
*/
import "C"
