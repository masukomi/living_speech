package openvox

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// readSpeechEvents parses the server-sent event stream from /audio/speech.
// Events: response.created, audio.chunk (data.audio = base64 WAV), response.completed.
func readSpeechEvents(r io.Reader, onChunk func([]byte)) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 64*1024*1024)

	var event string
	var data strings.Builder
	completed := false

	dispatch := func() error {
		defer func() { event = ""; data.Reset() }()
		if data.Len() == 0 {
			return nil
		}
		var payload struct {
			Type  string `json:"type"`
			Audio string `json:"audio"`
			Data  *struct {
				Audio string `json:"audio"`
			} `json:"data"`
			Error any `json:"error"`
		}
		if err := json.Unmarshal([]byte(data.String()), &payload); err != nil {
			return nil // ignore non-JSON keepalives etc.
		}
		name := event
		if name == "" {
			name = payload.Type
		}
		switch name {
		case "audio.chunk":
			b64 := payload.Audio
			if payload.Data != nil && payload.Data.Audio != "" {
				b64 = payload.Data.Audio
			}
			if b64 == "" {
				return nil
			}
			audio, err := base64.StdEncoding.DecodeString(b64)
			if err != nil {
				return fmt.Errorf("openvox: bad audio chunk: %w", err)
			}
			onChunk(audio)
		case "response.completed":
			completed = true
		case "error", "response.failed", "response.error":
			return fmt.Errorf("openvox: speech failed: %v", payload.Error)
		}
		return nil
	}

	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "":
			if err := dispatch(); err != nil {
				return err
			}
			if completed {
				return nil
			}
		case strings.HasPrefix(line, ":"):
			// comment
		case strings.HasPrefix(line, "event:"):
			event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	if err := sc.Err(); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return dispatch()
}
