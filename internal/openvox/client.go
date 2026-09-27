// Package openvox is a small client for the OpenVox local speech API.
package openvox

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const DefaultBaseURL = "http://127.0.0.1:8000/v1"

// ErrVoiceNotFound is returned by Speak when the server rejects the voice.
var ErrVoiceNotFound = errors.New("openvox: voice not found")

// Client talks to an OpenVox server.
type Client struct {
	BaseURL string
	// HTTP is used for quick metadata calls.
	HTTP *http.Client
	// SpeechHTTP is used for speech generation and model loading, which can be slow.
	SpeechHTTP *http.Client
	// MaxBusyWait bounds how long we retry on 429 responses.
	MaxBusyWait time.Duration
	// busyBackoff is the initial 429 retry delay (overridable in tests).
	busyBackoff time.Duration
}

func New(baseURL string) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{
		BaseURL:     strings.TrimRight(baseURL, "/"),
		HTTP:        &http.Client{Timeout: 5 * time.Second},
		SpeechHTTP:  &http.Client{},
		MaxBusyWait: 60 * time.Second,
		busyBackoff: 500 * time.Millisecond,
	}
}

// Option is a selectable item (model, language, or voice).
type Option struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// SpeechRequest is the body of POST /audio/speech.
type SpeechRequest struct {
	Model          string `json:"model"`
	Input          string `json:"input"`
	Language       string `json:"language,omitempty"`
	Voice          string `json:"voice,omitempty"`
	ResponseFormat string `json:"response_format,omitempty"`
	Stream         bool   `json:"stream"`
}

// APIError is a non-2xx response.
type APIError struct {
	Status int
	Body   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("openvox: HTTP %d: %s", e.Status, e.Body)
}

func (c *Client) Models(ctx context.Context) ([]Option, error) {
	return c.list(ctx, "/models", "models")
}

func (c *Client) Languages(ctx context.Context, model string) ([]Option, error) {
	return c.list(ctx, "/models/"+url.PathEscape(model)+"/languages", "languages")
}

func (c *Client) Voices(ctx context.Context, model, language string) ([]Option, error) {
	p := "/models/" + url.PathEscape(model) + "/voices"
	if language != "" {
		p += "?language=" + url.QueryEscape(language)
	}
	return c.list(ctx, p, "voices")
}

// Load warms a model. It retries while the server is busy.
func (c *Client) Load(ctx context.Context, model string) error {
	resp, err := c.doWithRetry(ctx, c.SpeechHTTP, func() (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/models/"+url.PathEscape(model)+"/load", nil)
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

// Speak generates speech, calling onChunk with each WAV segment as it arrives.
// Streaming (SSE) responses yield many chunks; a plain audio response yields one.
func (c *Client) Speak(ctx context.Context, req SpeechRequest, onChunk func([]byte)) error {
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	resp, err := c.doWithRetry(ctx, c.SpeechHTTP, func() (*http.Request, error) {
		r, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/audio/speech", bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		r.Header.Set("Content-Type", "application/json")
		if req.Stream {
			r.Header.Set("Accept", "text/event-stream")
		}
		return r, nil
	})
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && isVoiceError(apiErr) {
			return fmt.Errorf("%w: %s", ErrVoiceNotFound, apiErr.Body)
		}
		return err
	}
	defer resp.Body.Close()

	if strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		return readSpeechEvents(resp.Body, onChunk)
	}
	audio, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if len(audio) > 0 {
		onChunk(audio)
	}
	return nil
}

// isVoiceError detects OpenVox's "Voice not found" error (returned as a 500).
func isVoiceError(e *APIError) bool {
	body := strings.ToLower(e.Body)
	return e.Status >= 400 && strings.Contains(body, "voice") && strings.Contains(body, "not found")
}

func (c *Client) list(ctx context.Context, path, key string) ([]Option, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode/100 != 2 {
		return nil, &APIError{Status: resp.StatusCode, Body: strings.TrimSpace(string(data))}
	}
	return decodeOptions(data, key)
}

// doWithRetry performs a request, retrying with backoff on 429 (server busy).
func (c *Client) doWithRetry(ctx context.Context, hc *http.Client, build func() (*http.Request, error)) (*http.Response, error) {
	delay := c.busyBackoff
	deadline := time.Now().Add(c.MaxBusyWait)
	for {
		req, err := build()
		if err != nil {
			return nil, err
		}
		resp, err := hc.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode/100 == 2 {
			return resp, nil
		}
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusTooManyRequests || time.Now().Add(delay).After(deadline) {
			return nil, &APIError{Status: resp.StatusCode, Body: strings.TrimSpace(string(data))}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
		delay = min(delay*2, 4*time.Second)
	}
}

// decodeOptions accepts the list shapes an OpenAI-style API is likely to use:
// a bare array, or an object wrapping the array under "data" or the resource key.
// Array items may be strings or objects with an id/name/code field.
func decodeOptions(data []byte, key string) ([]Option, error) {
	var raw json.RawMessage = data
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err == nil {
		found := false
		for _, k := range []string{key, "data", "items", "results"} {
			if v, ok := obj[k]; ok {
				raw, found = v, true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("openvox: unexpected %s response: %s", key, truncate(string(data), 200))
		}
	}

	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		// Some APIs return a map of id -> details.
		var m map[string]json.RawMessage
		if err2 := json.Unmarshal(raw, &m); err2 != nil {
			return nil, fmt.Errorf("openvox: unexpected %s response: %s", key, truncate(string(data), 200))
		}
		out := make([]Option, 0, len(m))
		for id := range m {
			out = append(out, Option{ID: id, Name: id})
		}
		return out, nil
	}

	out := make([]Option, 0, len(items))
	for _, it := range items {
		if o, ok := decodeOption(it); ok {
			out = append(out, o)
		}
	}
	return out, nil
}

func decodeOption(it json.RawMessage) (Option, bool) {
	var s string
	if json.Unmarshal(it, &s) == nil {
		return Option{ID: s, Name: s}, s != ""
	}
	var m map[string]any
	if json.Unmarshal(it, &m) != nil {
		return Option{}, false
	}
	str := func(keys ...string) string {
		for _, k := range keys {
			if v, ok := m[k].(string); ok && v != "" {
				return v
			}
		}
		return ""
	}
	id := str("id", "code", "voice_id", "language", "name")
	name := str("name", "display_name", "label", "id", "code")
	if name == "" {
		name = id
	}
	return Option{ID: id, Name: name}, id != ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
