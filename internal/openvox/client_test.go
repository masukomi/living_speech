package openvox

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestDecodeOptionsShapes(t *testing.T) {
	cases := map[string]string{
		"openai data":  `{"object":"list","data":[{"id":"kokoro"},{"id":"chatterbox"}]}`,
		"keyed":        `{"models":[{"id":"kokoro","name":"Kokoro"},{"id":"chatterbox"}]}`,
		"bare strings": `["kokoro","chatterbox"]`,
		"bare objects": `[{"name":"kokoro"},{"code":"chatterbox"}]`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := decodeOptions([]byte(body), "models")
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != 2 || got[0].ID != "kokoro" || got[1].ID != "chatterbox" {
				t.Fatalf("got %+v", got)
			}
		})
	}
	if _, err := decodeOptions([]byte(`{"unexpected":1}`), "models"); err == nil {
		t.Fatal("expected error for unknown shape")
	}
}

func TestListEndpoints(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			fmt.Fprint(w, `{"data":[{"id":"kokoro"}]}`)
		case "/v1/models/kokoro/languages":
			fmt.Fprint(w, `{"languages":[{"code":"en","name":"English"}]}`)
		case "/v1/models/kokoro/voices":
			if r.URL.Query().Get("language") != "en" {
				http.Error(w, "missing language", 400)
				return
			}
			fmt.Fprint(w, `{"voices":["af_bella","am_adam"]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := New(srv.URL + "/v1")
	ctx := context.Background()

	langs, err := c.Languages(ctx, "kokoro")
	if err != nil || len(langs) != 1 || langs[0].ID != "en" || langs[0].Name != "English" {
		t.Fatalf("languages: %+v %v", langs, err)
	}
	voices, err := c.Voices(ctx, "kokoro", "en")
	if err != nil || len(voices) != 2 || voices[0].ID != "af_bella" {
		t.Fatalf("voices: %+v %v", voices, err)
	}
	if _, err := c.Languages(ctx, "nope"); err == nil {
		t.Fatal("expected 404 error")
	}
}

func sseEvent(name string, data any) string {
	b, _ := json.Marshal(data)
	return fmt.Sprintf("event: %s\ndata: %s\n\n", name, b)
}

func TestSpeakStreaming(t *testing.T) {
	chunks := [][]byte{[]byte("RIFF-one"), []byte("RIFF-two")}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req SpeechRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if !req.Stream || req.Voice != "af_bella" || req.Language != "en" {
			http.Error(w, "bad request", 400)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, ": keepalive\n\n")
		fmt.Fprint(w, sseEvent("response.created", map[string]any{"id": "r1"}))
		for _, c := range chunks {
			fmt.Fprint(w, sseEvent("audio.chunk", map[string]any{"data": map[string]string{"audio": base64.StdEncoding.EncodeToString(c)}}))
		}
		fmt.Fprint(w, sseEvent("response.completed", map[string]any{}))
	}))
	defer srv.Close()

	var got []string
	err := New(srv.URL).SpeakWithProgress(context.Background(), SpeechRequest{Model: "kokoro", Input: "hi", Language: "en", Voice: "af_bella", Stream: true},
		func() { got = append(got, "accepted") },
		func(b []byte) { got = append(got, string(b)) })
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != "accepted,RIFF-one,RIFF-two" {
		t.Fatalf("chunks: %v", got)
	}
}

func TestSpeakNonStreamingFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/wav")
		fmt.Fprint(w, "RIFF-whole")
	}))
	defer srv.Close()
	var got []string
	if err := New(srv.URL).Speak(context.Background(), SpeechRequest{Stream: true}, func(b []byte) { got = append(got, string(b)) }); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "RIFF-whole" {
		t.Fatalf("got %v", got)
	}
}

func TestBusyRetry(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 3 {
			http.Error(w, "busy", http.StatusTooManyRequests)
			return
		}
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer srv.Close()
	c := New(srv.URL)
	c.busyBackoff = time.Millisecond
	if err := c.Load(context.Background(), "kokoro"); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 {
		t.Fatalf("calls = %d", calls.Load())
	}
}

func TestSpeakVoiceNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"Voice not found: gone","type":"invalid_request_error"}}`, http.StatusInternalServerError)
	}))
	defer srv.Close()
	err := New(srv.URL).Speak(context.Background(), SpeechRequest{Voice: "gone"}, func([]byte) {})
	if !errors.Is(err, ErrVoiceNotFound) {
		t.Fatalf("err = %v", err)
	}
}
