package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

// Resize the window while keeping its top-left corner where it is (Cocoa's
// default is to keep the bottom-left). If that would push it off the bottom or
// right of the screen, slide it just enough to keep it visible.
static void setSizeKeepingTopLeft(void* nsWindow, int width, int height) {
	NSWindow* window = (NSWindow*)nsWindow;
	NSRect frame = [window frame];
	CGFloat top = NSMaxY(frame);
	frame.size.width = width;
	frame.size.height = height;
	frame.origin.y = top - height;

	NSScreen* screen = [window screen] ?: [NSScreen mainScreen];
	NSRect visible = [screen visibleFrame];
	if (frame.origin.y < NSMinY(visible)) {
		frame.origin.y = MIN(NSMinY(visible), NSMaxY(visible) - height);
	}
	if (NSMaxX(frame) > NSMaxX(visible)) {
		frame.origin.x = MAX(NSMinX(visible), NSMaxX(visible) - width);
	}
	[window setFrame:frame display:YES animate:NO];
}
*/
import "C"

import "github.com/wailsapp/wails/v3/pkg/application"

func setSizeKeepingTopLeft(window *application.WebviewWindow, width, height int) {
	application.InvokeSync(func() {
		if ptr := window.NativeWindow(); ptr != nil {
			C.setSizeKeepingTopLeft(ptr, C.int(width), C.int(height))
		}
	})
}
