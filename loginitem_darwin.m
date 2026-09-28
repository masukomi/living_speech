// Registers LivingSpeech to open at login using the public SMAppService API.
// The user can also see and change this in System Settings › General › Login
// Items, so the system is the source of truth, not our settings.

#import <Foundation/Foundation.h>
#import <ServiceManagement/ServiceManagement.h>
#include <stdlib.h>
#include <string.h>

// Returns an SMAppServiceStatus: 0 not registered, 1 enabled,
// 2 requires approval, 3 not found.
int loginItemStatus(void) {
	return (int)[SMAppService mainAppService].status;
}

// Turns open-at-login on or off. Returns NULL on success, or an error message
// the caller must free.
char *loginItemSetEnabled(int enable) {
	SMAppService *service = [SMAppService mainAppService];
	NSError *error = nil;
	BOOL ok = enable ? [service registerAndReturnError:&error]
	                 : [service unregisterAndReturnError:&error];
	if (ok) {
		return NULL;
	}
	return strdup(error.localizedDescription.UTF8String ?: "unknown error");
}

void loginItemOpenSettings(void) {
	[SMAppService openSystemSettingsLoginItems];
}
