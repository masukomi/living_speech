package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

// Resize the window to `height` points while keeping its top edge where it is
// (Cocoa's default is to keep the bottom edge). If that would push the bottom
// off the screen, slide the window up just enough to keep it visible.
static void setHeightKeepingTop(void* nsWindow, int height) {
	NSWindow* window = (NSWindow*)nsWindow;
	NSRect frame = [window frame];
	CGFloat top = NSMaxY(frame);
	frame.size.height = height;
	frame.origin.y = top - height;

	NSScreen* screen = [window screen] ?: [NSScreen mainScreen];
	NSRect visible = [screen visibleFrame];
	if (frame.origin.y < NSMinY(visible)) {
		frame.origin.y = MIN(NSMinY(visible), NSMaxY(visible) - height);
	}
	[window setFrame:frame display:YES animate:NO];
}
*/
import "C"

import "github.com/wailsapp/wails/v3/pkg/application"

func setHeightKeepingTop(window *application.WebviewWindow, height int) {
	application.InvokeSync(func() {
		if ptr := window.NativeWindow(); ptr != nil {
			C.setHeightKeepingTop(ptr, C.int(height))
		}
	})
}
