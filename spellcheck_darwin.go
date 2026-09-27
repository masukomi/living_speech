package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation
#import <Foundation/Foundation.h>

// WKWebView reads this default to decide whether to underline misspellings
// as you type. Registering (rather than setting) it keeps any choice the user
// makes via the context menu's "Check Spelling While Typing" toggle.
static void enableContinuousSpellChecking(void) {
	[[NSUserDefaults standardUserDefaults] registerDefaults:@{
		@"WebContinuousSpellCheckingEnabled": @YES,
	}];
}
*/
import "C"

// enableSpellChecking turns on the system spell checker in the webview. It
// must run before the webview is created.
func enableSpellChecking() {
	C.enableContinuousSpellChecking()
}
