package app

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa -framework WebKit
#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>

// Native browser views for the Preview tab (browser.go). A preview site in
// an <iframe> is a third-party frame under the wails:// top origin, so
// WebKit drops its cookies and logins loop back to the sign-in page. Each
// BishBrowser is instead its own top-level WKWebView on the persistent
// default data store, laid over the Preview tab's stage as a sibling of the
// Wails webview. All AppKit work hops to the main queue: these are called
// from Wails' binding goroutines.

extern void bishBrowserNavGo(char* bid, char* url, char* title, int loading, int canBack, int canFwd);
extern void bishBrowserKeyGo(char* bid, char* json);

static NSString* const kBishBrowserIdent = @"bish-browser";
static NSString* const kBishKeyHandler = @"bishKey";

// Cmd-combos go up to bish's own keybinds (⌘P, ⌘W, ⌘⇧[ …) — the native
// view has keyboard focus, so bish's window keydown listener never sees
// them otherwise. Clipboard/undo/select-all stay with the page.
static NSString* const kBishKeyScript =
    @"addEventListener('keydown', function (e) {"
     "  if (!e.metaKey) return;"
     "  var k = e.key.toLowerCase();"
     "  if (!e.shiftKey && !e.altKey && k.length === 1 && 'cvxaz'.indexOf(k) >= 0) return;"
     "  window.webkit.messageHandlers.bishKey.postMessage(JSON.stringify({key: e.key, code: e.code,"
     "    metaKey: e.metaKey, ctrlKey: e.ctrlKey, shiftKey: e.shiftKey, altKey: e.altKey}));"
     "}, true);";

@interface BishBrowser : NSObject <WKNavigationDelegate, WKUIDelegate, WKScriptMessageHandler>
@property (copy) NSString* bid;
@property (retain) WKWebView* view;
@property (retain) NSMutableArray<NSWindow*>* popups;
@end

static NSMutableDictionary<NSString*, BishBrowser*>* bishBrowsers(void) {
    static NSMutableDictionary* d = nil;
    if (!d) d = [[NSMutableDictionary alloc] init];
    return d;
}

static NSArray<NSString*>* bishObservedKeys(void) {
    return @[@"URL", @"title", @"loading", @"canGoBack", @"canGoForward"];
}

// The Wails webview: the WKWebView that isn't one of ours.
static WKWebView* bishHostWebView(void) {
    NSMutableArray<NSWindow*>* wins = [NSMutableArray array];
    if ([NSApp mainWindow]) [wins addObject:[NSApp mainWindow]];
    [wins addObjectsFromArray:[NSApp windows]];
    for (NSWindow* win in wins) {
        for (NSView* v in [[win contentView] subviews]) {
            if ([v isKindOfClass:[WKWebView class]] && ![[v identifier] isEqualToString:kBishBrowserIdent]) {
                return (WKWebView*)v;
            }
        }
    }
    return nil;
}

@implementation BishBrowser

- (void)emitNav {
    WKWebView* v = self.view;
    if (!v) return;
    NSString* url = [[v URL] absoluteString] ?: @"";
    NSString* title = [v title] ?: @"";
    bishBrowserNavGo((char*)[self.bid UTF8String], (char*)[url UTF8String], (char*)[title UTF8String],
                     [v isLoading] ? 1 : 0, [v canGoBack] ? 1 : 0, [v canGoForward] ? 1 : 0);
}

// KVO on URL too, not just navigation callbacks: SPA pushState route
// changes never fire a navigation delegate method.
- (void)observeValueForKeyPath:(NSString*)keyPath ofObject:(id)object change:(NSDictionary*)change context:(void*)context {
    [self emitNav];
}

- (void)userContentController:(WKUserContentController*)ucc didReceiveScriptMessage:(WKScriptMessage*)message {
    if (![message.body isKindOfClass:[NSString class]]) return;
    bishBrowserKeyGo((char*)[self.bid UTF8String], (char*)[(NSString*)message.body UTF8String]);
}

- (void)webView:(WKWebView*)webView didFailProvisionalNavigation:(WKNavigation*)navigation withError:(NSError*)error {
    [self emitNav];
}

// window.open / target=_blank. A plain link click loads in place; a real
// popup (OAuth sign-in windows) gets a panel built from the configuration
// WebKit hands us, so window.opener and postMessage back still work.
- (WKWebView*)webView:(WKWebView*)webView createWebViewWithConfiguration:(WKWebViewConfiguration*)configuration
       forNavigationAction:(WKNavigationAction*)action windowFeatures:(WKWindowFeatures*)features {
    if (action.navigationType == WKNavigationTypeLinkActivated && features.width == nil) {
        [self.view loadRequest:action.request];
        return nil;
    }
    CGFloat w = features.width ? [features.width doubleValue] : 520;
    CGFloat h = features.height ? [features.height doubleValue] : 680;
    NSWindow* host = [self.view window];
    NSRect hf = host ? [host frame] : NSMakeRect(0, 0, w, h);
    NSRect r = NSMakeRect(NSMidX(hf) - w / 2, NSMidY(hf) - h / 2, w, h);
    NSWindow* panel = [[NSWindow alloc] initWithContentRect:r
                                                  styleMask:NSWindowStyleMaskTitled | NSWindowStyleMaskClosable | NSWindowStyleMaskResizable
                                                    backing:NSBackingStoreBuffered
                                                      defer:NO];
    [panel setReleasedWhenClosed:NO];
    WKWebView* pv = [[[WKWebView alloc] initWithFrame:NSMakeRect(0, 0, w, h) configuration:configuration] autorelease];
    pv.UIDelegate = self;
    [panel setContentView:pv];
    [panel setTitle:@"Sign in"];
    [panel makeKeyAndOrderFront:nil];
    [self.popups addObject:panel];
    [panel release];
    return pv;
}

- (void)webViewDidClose:(WKWebView*)webView {
    for (NSWindow* p in [[self.popups copy] autorelease]) {
        if ([p contentView] == webView) {
            [p close];
            [self.popups removeObject:p];
        }
    }
}

- (void)webView:(WKWebView*)webView runJavaScriptAlertPanelWithMessage:(NSString*)message
    initiatedByFrame:(WKFrameInfo*)frame completionHandler:(void (^)(void))completionHandler {
    NSAlert* a = [[[NSAlert alloc] init] autorelease];
    [a setMessageText:message];
    [a runModal];
    completionHandler();
}

- (void)webView:(WKWebView*)webView runJavaScriptConfirmPanelWithMessage:(NSString*)message
    initiatedByFrame:(WKFrameInfo*)frame completionHandler:(void (^)(BOOL))completionHandler {
    NSAlert* a = [[[NSAlert alloc] init] autorelease];
    [a setMessageText:message];
    [a addButtonWithTitle:@"OK"];
    [a addButtonWithTitle:@"Cancel"];
    completionHandler([a runModal] == NSAlertFirstButtonReturn);
}

- (void)webView:(WKWebView*)webView runOpenPanelWithParameters:(WKOpenPanelParameters*)parameters
    initiatedByFrame:(WKFrameInfo*)frame completionHandler:(void (^)(NSArray<NSURL*>*))completionHandler {
    NSOpenPanel* p = [NSOpenPanel openPanel];
    [p setAllowsMultipleSelection:parameters.allowsMultipleSelection];
    [p setCanChooseDirectories:parameters.allowsDirectories];
    completionHandler([p runModal] == NSModalResponseOK ? [p URLs] : nil);
}

@end

void bishBrowserOpenC(char* cbid, char* curl) {
    NSString* bid = [NSString stringWithUTF8String:cbid];
    NSString* url = [NSString stringWithUTF8String:curl];
    dispatch_async(dispatch_get_main_queue(), ^{
        if (bishBrowsers()[bid]) return;
        WKWebView* host = bishHostWebView();
        if (!host) return;

        WKWebViewConfiguration* cfg = [[[WKWebViewConfiguration alloc] init] autorelease];
        cfg.websiteDataStore = [WKWebsiteDataStore defaultDataStore];
        // Google refuses sign-in from a bare WKWebView user agent
        cfg.applicationNameForUserAgent = @"Version/17.0 Safari/605.1.15";
        [cfg.preferences setValue:@YES forKey:@"developerExtrasEnabled"];

        BishBrowser* b = [[[BishBrowser alloc] init] autorelease];
        b.bid = bid;
        b.popups = [NSMutableArray array];
        [cfg.userContentController addScriptMessageHandler:b name:kBishKeyHandler];
        WKUserScript* ks = [[[WKUserScript alloc] initWithSource:kBishKeyScript
                                                  injectionTime:WKUserScriptInjectionTimeAtDocumentStart
                                               forMainFrameOnly:YES] autorelease];
        [cfg.userContentController addUserScript:ks];

        WKWebView* v = [[[WKWebView alloc] initWithFrame:NSZeroRect configuration:cfg] autorelease];
        [v setIdentifier:kBishBrowserIdent];
        v.navigationDelegate = b;
        v.UIDelegate = b;
        [v setHidden:YES];
#if __MAC_OS_X_VERSION_MAX_ALLOWED >= 130300
        if (@available(macOS 13.3, *)) { v.inspectable = YES; }
#endif
        b.view = v;
        for (NSString* k in bishObservedKeys()) {
            [v addObserver:b forKeyPath:k options:NSKeyValueObservingOptionNew context:NULL];
        }
        [[host superview] addSubview:v positioned:NSWindowAbove relativeTo:host];
        bishBrowsers()[bid] = b;

        NSURL* u = [NSURL URLWithString:url];
        if (u) [v loadRequest:[NSURLRequest requestWithURL:u]];
    });
}

// x/y/w/h are CSS px from getBoundingClientRect in the Wails webview.
void bishBrowserSetFrameC(char* cbid, double x, double y, double w, double h) {
    NSString* bid = [NSString stringWithUTF8String:cbid];
    dispatch_async(dispatch_get_main_queue(), ^{
        BishBrowser* b = bishBrowsers()[bid];
        WKWebView* host = bishHostWebView();
        if (!b || !host) return;
        CGFloat z = 1;
        if (@available(macOS 11.0, *)) { z = host.pageZoom; }
        NSRect r = NSMakeRect(x * z, y * z, w * z, h * z);
        if (![host isFlipped]) r.origin.y = NSHeight([host bounds]) - r.origin.y - r.size.height;
        [b.view setFrame:[host convertRect:r toView:[b.view superview]]];
    });
}

void bishBrowserSetVisibleC(char* cbid, int visible) {
    NSString* bid = [NSString stringWithUTF8String:cbid];
    dispatch_async(dispatch_get_main_queue(), ^{
        BishBrowser* b = bishBrowsers()[bid];
        if (!b) return;
        [b.view setHidden:!visible];
        // hiding the first responder strands keyboard focus in a hidden
        // view — hand it back to bish
        if (!visible && [[b.view window] firstResponder] == b.view) {
            WKWebView* host = bishHostWebView();
            if (host) [[host window] makeFirstResponder:host];
        }
    });
}

void bishBrowserNavigateC(char* cbid, char* curl) {
    NSString* bid = [NSString stringWithUTF8String:cbid];
    NSString* url = [NSString stringWithUTF8String:curl];
    dispatch_async(dispatch_get_main_queue(), ^{
        BishBrowser* b = bishBrowsers()[bid];
        NSURL* u = [NSURL URLWithString:url];
        if (b && u) [b.view loadRequest:[NSURLRequest requestWithURL:u]];
    });
}

// op: 0 reload, 1 back, 2 forward
void bishBrowserCmdC(char* cbid, int op) {
    NSString* bid = [NSString stringWithUTF8String:cbid];
    dispatch_async(dispatch_get_main_queue(), ^{
        BishBrowser* b = bishBrowsers()[bid];
        if (!b) return;
        if (op == 0) [b.view reload];
        else if (op == 1) [b.view goBack];
        else if (op == 2) [b.view goForward];
    });
}

void bishBrowserCloseC(char* cbid) {
    NSString* bid = [NSString stringWithUTF8String:cbid];
    dispatch_async(dispatch_get_main_queue(), ^{
        BishBrowser* b = bishBrowsers()[bid];
        if (!b) return;
        WKWebView* v = b.view;
        for (NSString* k in bishObservedKeys()) [v removeObserver:b forKeyPath:k];
        // the script message handler retains b — break the cycle
        [v.configuration.userContentController removeScriptMessageHandlerForName:kBishKeyHandler];
        v.navigationDelegate = nil;
        v.UIDelegate = nil;
        [v stopLoading];
        [v removeFromSuperview];
        for (NSWindow* p in b.popups) [p close];
        b.view = nil;
        [bishBrowsers() removeObjectForKey:bid];
    });
}
*/
import "C"
