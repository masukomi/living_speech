package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework AppKit
#include <stdlib.h>

void systemSpeechStart(int utteranceID, const char *text);
void systemSpeechStop(int utteranceID);
*/
import "C"

import (
	"context"
	"errors"
	"sync"
	"time"
	"unsafe"
)

// systemUtterance tracks one NSSpeechSynthesizer utterance's callbacks.
type systemUtterance struct {
	started  chan struct{}
	finished chan bool // true if it spoke to the end, false if stopped or failed
}

var (
	systemMu         sync.Mutex
	systemUtterances = map[int]*systemUtterance{}
)

//export goSystemSpeechStarted
func goSystemSpeechStarted(id C.int) {
	systemMu.Lock()
	u := systemUtterances[int(id)]
	systemMu.Unlock()
	if u != nil {
		close(u.started)
	}
}

//export goSystemSpeechFinished
func goSystemSpeechFinished(id C.int, ok C.int) {
	systemMu.Lock()
	u := systemUtterances[int(id)]
	delete(systemUtterances, int(id))
	systemMu.Unlock()
	if u != nil {
		u.finished <- ok != 0
	}
}

// speakWithSystemVoice speaks text through the Mac's speakers with the System
// Voice. onStarted runs when audio begins. It returns when speech ends, or
// stops speech and returns ctx's error if ctx ends first.
func speakWithSystemVoice(ctx context.Context, id int, text string, onStarted func()) error {
	u := &systemUtterance{started: make(chan struct{}), finished: make(chan bool, 1)}
	systemMu.Lock()
	systemUtterances[id] = u
	systemMu.Unlock()

	ctext := C.CString(text)
	defer C.free(unsafe.Pointer(ctext))
	C.systemSpeechStart(C.int(id), ctext)

	started := u.started
	for {
		select {
		case <-started:
			started = nil // fire once
			onStarted()
		case ok := <-u.finished:
			if !ok && ctx.Err() == nil {
				return errors.New("the system voice stopped unexpectedly")
			}
			return ctx.Err()
		case <-ctx.Done():
			C.systemSpeechStop(C.int(id))
			// Wait for the synthesizer to acknowledge, but don't hang if it never does.
			select {
			case <-u.finished:
			case <-time.After(2 * time.Second):
				systemMu.Lock()
				delete(systemUtterances, id)
				systemMu.Unlock()
			}
			return ctx.Err()
		}
	}
}
