package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation
#import <Foundation/Foundation.h>
#include <stdlib.h>
#include <string.h>

// The app bundle's CFBundleShortVersionString, or NULL when not running from
// a bundle (e.g. a bare `go build` binary). The caller must free the result.
static char *bundleVersion(void) {
	NSString *v = [[NSBundle mainBundle] objectForInfoDictionaryKey:@"CFBundleShortVersionString"];
	return v ? strdup(v.UTF8String) : NULL;
}
*/
import "C"

import "unsafe"

// appVersion is the version from the app bundle's Info.plist.
func appVersion() string {
	v := C.bundleVersion()
	if v == nil {
		return "unknown (not running from an app bundle)"
	}
	defer C.free(unsafe.Pointer(v))
	return C.GoString(v)
}
