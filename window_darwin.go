//go:build darwin

package main

/*
#cgo darwin CFLAGS: -x objective-c
#cgo darwin LDFLAGS: -framework Cocoa -framework WebKit

#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>

@interface LabWindowDelegate : NSObject <NSWindowDelegate>
@end

@implementation LabWindowDelegate
- (void)windowWillClose:(NSNotification *)notification {
    [NSApp terminate:nil];
}
@end

void setupMainMenu() {
    NSMenu *mainMenu = [[NSMenu alloc] init];

    // App Menu
    NSMenuItem *appMenuItem = [[NSMenuItem alloc] init];
    [mainMenu addItem:appMenuItem];
    NSMenu *appMenu = [[NSMenu alloc] initWithTitle:@"Ollama Lab"];
    [appMenu addItemWithTitle:@"Hide Ollama Lab" action:@selector(hide:) keyEquivalent:@"h"];
    [appMenu addItem:[NSMenuItem separatorItem]];
    [appMenu addItemWithTitle:@"Quit Ollama Lab" action:@selector(terminate:) keyEquivalent:@"q"];
    [appMenuItem setSubmenu:appMenu];

    // Edit Menu (Crucial for Copy, Paste, Cut, Select All, Undo)
    NSMenuItem *editMenuItem = [[NSMenuItem alloc] init];
    [mainMenu addItem:editMenuItem];
    NSMenu *editMenu = [[NSMenu alloc] initWithTitle:@"Edit"];
    [editMenu addItemWithTitle:@"Undo" action:@selector(undo:) keyEquivalent:@"z"];
    [editMenu addItemWithTitle:@"Redo" action:@selector(redo:) keyEquivalent:@"Z"];
    [editMenu addItem:[NSMenuItem separatorItem]];
    [editMenu addItemWithTitle:@"Cut" action:@selector(cut:) keyEquivalent:@"x"];
    [editMenu addItemWithTitle:@"Copy" action:@selector(copy:) keyEquivalent:@"c"];
    [editMenu addItemWithTitle:@"Paste" action:@selector(paste:) keyEquivalent:@"v"];
    [editMenu addItemWithTitle:@"Select All" action:@selector(selectAll:) keyEquivalent:@"a"];
    [editMenuItem setSubmenu:editMenu];

    // Window Menu
    NSMenuItem *windowMenuItem = [[NSMenuItem alloc] init];
    [mainMenu addItem:windowMenuItem];
    NSMenu *windowMenu = [[NSMenu alloc] initWithTitle:@"Window"];
    [windowMenu addItemWithTitle:@"Minimize" action:@selector(performMiniaturize:) keyEquivalent:@"m"];
    [windowMenu addItemWithTitle:@"Zoom" action:@selector(performZoom:) keyEquivalent:@""];
    [windowMenu addItem:[NSMenuItem separatorItem]];
    [windowMenu addItemWithTitle:@"Close Window" action:@selector(performClose:) keyEquivalent:@"w"];
    [windowMenuItem setSubmenu:windowMenu];

    [NSApp setMainMenu:mainMenu];
}

void launchNativeWindow(const char* title, const char* urlStr, int width, int height) {
    @autoreleasepool {
        [NSApplication sharedApplication];
        [NSApp setActivationPolicy:NSApplicationActivationPolicyRegular];
        setupMainMenu();

        NSRect frame = NSMakeRect(0, 0, width, height);
        NSWindow *window = [[NSWindow alloc] initWithContentRect:frame
            styleMask:(NSWindowStyleMaskTitled | NSWindowStyleMaskClosable | NSWindowStyleMaskMiniaturizable | NSWindowStyleMaskResizable)
            backing:NSBackingStoreBuffered
            defer:NO];

        [window setTitle:[NSString stringWithUTF8String:title]];
        [window setMinSize:NSMakeSize(900, 600)];
        [window center];

        LabWindowDelegate *delegate = [[LabWindowDelegate alloc] init];
        [window setDelegate:delegate];

        WKWebViewConfiguration *config = [[WKWebViewConfiguration alloc] init];
        // Allow developer tools inspection if needed
        [[config preferences] setValue:@YES forKey:@"developerExtrasEnabled"];

        WKWebView *webView = [[WKWebView alloc] initWithFrame:frame configuration:config];
        webView.autoresizingMask = NSViewWidthSizable | NSViewHeightSizable;

        NSURL *url = [NSURL URLWithString:[NSString stringWithUTF8String:urlStr]];
        NSURLRequest *request = [NSURLRequest requestWithURL:url];
        [webView loadRequest:request];

        [window setContentView:webView];
        [window makeKeyAndOrderFront:nil];
        [NSApp activateIgnoringOtherApps:YES];
        [NSApp run];
    }
}
*/
import "C"
import (
	"unsafe"
)

func runDesktopWindow(title, url string, width, height int) {
	cTitle := C.CString(title)
	cUrl := C.CString(url)
	defer C.free(unsafe.Pointer(cTitle))
	defer C.free(unsafe.Pointer(cUrl))

	C.launchNativeWindow(cTitle, cUrl, C.int(width), C.int(height))
}
