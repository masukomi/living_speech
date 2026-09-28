package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation -framework ServiceManagement
#include <stdlib.h>

int loginItemStatus(void);
char *loginItemSetEnabled(int enable);
void loginItemOpenSettings(void);
*/
import "C"

import (
	"errors"
	"unsafe"
)

// LaunchAtLogin describes whether LivingSpeech opens when you log in.
type LaunchAtLogin struct {
	Enabled bool `json:"enabled"`
	// NeedsApproval means it's registered but the user must allow it in
	// System Settings › General › Login Items before it takes effect.
	NeedsApproval bool `json:"needsApproval"`
}

// SMAppServiceStatus values.
const (
	loginStatusEnabled         = 1
	loginStatusRequiresApprove = 2
)

func launchAtLoginState() LaunchAtLogin {
	switch C.loginItemStatus() {
	case loginStatusEnabled:
		return LaunchAtLogin{Enabled: true}
	case loginStatusRequiresApprove:
		return LaunchAtLogin{Enabled: true, NeedsApproval: true}
	default: // not registered, or not found
		return LaunchAtLogin{}
	}
}

func setLaunchAtLogin(on bool) error {
	enable := C.int(0)
	if on {
		enable = 1
	}
	if msg := C.loginItemSetEnabled(enable); msg != nil {
		defer C.free(unsafe.Pointer(msg))
		return errors.New(C.GoString(msg))
	}
	return nil
}

func openLoginItemsSettings() {
	C.loginItemOpenSettings()
}
