package openvox

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

// Run with OPENVOX_LIVE=1 against a running OpenVox server.
func TestLiveServer(t *testing.T) {
	if os.Getenv("OPENVOX_LIVE") == "" {
		t.Skip("set OPENVOX_LIVE=1 to run against a local OpenVox")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	c := New(os.Getenv("OPENVOX_URL"))

	models, err := c.Models(ctx)
	if err != nil || len(models) == 0 {
		t.Fatalf("models: %v %v", models, err)
	}
	model := models[0].ID
	langs, err := c.Languages(ctx, model)
	if err != nil || len(langs) == 0 {
		t.Fatalf("languages: %v %v", langs, err)
	}
	voices, err := c.Voices(ctx, model, "en")
	if err != nil || len(voices) == 0 {
		t.Fatalf("voices: %v %v", voices, err)
	}
	t.Logf("model=%s (%s) langs=%d voices=%d first voice=%+v", model, models[0].Name, len(langs), len(voices), voices[0])

	if err := c.Load(ctx, model); err != nil {
		t.Fatalf("load: %v", err)
	}
	var chunks int
	err = c.Speak(ctx, SpeechRequest{Model: model, Input: "Testing one two three.", Language: "en", Voice: voices[0].ID, ResponseFormat: "wav", Stream: true},
		func(b []byte) {
			chunks++
			if !bytes.HasPrefix(b, []byte("RIFF")) {
				t.Errorf("chunk %d is not WAV", chunks)
			}
		})
	if err != nil || chunks == 0 {
		t.Fatalf("speak: chunks=%d err=%v", chunks, err)
	}

	err = c.Speak(ctx, SpeechRequest{Model: model, Input: "x", Language: "en", Voice: "no_such_voice", Stream: true}, func([]byte) {})
	if err == nil || !errors.Is(err, ErrVoiceNotFound) {
		t.Fatalf("expected ErrVoiceNotFound, got %v", err)
	}
}
