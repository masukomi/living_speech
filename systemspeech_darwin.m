// Speaks with the macOS System Voice (System Settings › Accessibility ›
// Spoken Content), using the public NSSpeechSynthesizer API. Creating the
// synthesizer with a nil voice picks up whatever the System Voice currently is.

#import <AppKit/AppKit.h>
#include "_cgo_export.h"

#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"

@interface LSSpeechDelegate : NSObject <NSSpeechSynthesizerDelegate>
@property (nonatomic) int utteranceID;
@property (nonatomic) BOOL started;
@end

@implementation LSSpeechDelegate
- (void)markStarted {
	if (!self.started) {
		self.started = YES;
		goSystemSpeechStarted(self.utteranceID);
	}
}
- (void)speechSynthesizer:(NSSpeechSynthesizer *)sender willSpeakWord:(NSRange)range ofString:(NSString *)string {
	[self markStarted];
}
- (void)speechSynthesizer:(NSSpeechSynthesizer *)sender willSpeakPhoneme:(short)phonemeOpcode {
	[self markStarted];
}
- (void)speechSynthesizer:(NSSpeechSynthesizer *)sender didFinishSpeaking:(BOOL)finishedSpeaking {
	goSystemSpeechFinished(self.utteranceID, finishedSpeaking ? 1 : 0);
}
@end

static NSSpeechSynthesizer *currentSynth;
static LSSpeechDelegate *currentDelegate;

void systemSpeechStart(int utteranceID, const char *text) {
	NSString *str = [NSString stringWithUTF8String:text];
	dispatch_async(dispatch_get_main_queue(), ^{
		[currentSynth stopSpeaking];
		// A fresh synthesizer each time so a changed System Voice is picked up.
		NSSpeechSynthesizer *synth = [[NSSpeechSynthesizer alloc] initWithVoice:nil];
		LSSpeechDelegate *delegate = [[LSSpeechDelegate alloc] init];
		delegate.utteranceID = utteranceID;
		synth.delegate = delegate;
		currentSynth = synth;
		currentDelegate = delegate;
		if (![synth startSpeakingString:str]) {
			goSystemSpeechFinished(utteranceID, 0);
		}
	});
}

// Stops the given utterance, if it's still the one speaking. (A newer
// utterance has already stopped it otherwise.)
void systemSpeechStop(int utteranceID) {
	dispatch_async(dispatch_get_main_queue(), ^{
		if (currentDelegate.utteranceID == utteranceID) {
			[currentSynth stopSpeaking];
		}
	});
}

#pragma clang diagnostic pop
