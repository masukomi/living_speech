package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"livingspeech/internal/openvox"
	"livingspeech/internal/store"
)

const (
	previewText = "Hello! This is how I sound."
	// speechTimeout bounds one OpenVox utterance, from pressing Return to the
	// end of the audio. For the system voice it bounds the wait for speech to
	// start, since audio then plays live for as long as the text takes.
	speechTimeout = 2 * time.Minute
	// systemTimingKey records response times for the system voice.
	systemTimingKey = "system"
)

var errStartTimeout = fmt.Errorf("the system voice didn't start within %v", speechTimeout)

// Event payloads sent to the frontend. ID identifies the utterance so the
// frontend can ignore chunks from a request it has already stopped.
type SpeechChunk struct {
	ID    int    `json:"id"`
	Audio string `json:"audio"` // base64 WAV
}

// SpeechProgress reports which stage a request is in: "sending" until OpenVox
// accepts it (including loading the model and waiting while it's busy), then
// "waiting" until the audio arrives.
type SpeechProgress struct {
	ID        int    `json:"id"`
	Stage     string `json:"stage"`
	TimeoutMs int    `json:"timeoutMs"`
}

// SpeechStarted reports that audio began playing natively (system voice),
// rather than arriving as speech:chunk events.
type SpeechStarted struct {
	ID int `json:"id"`
}

type SpeechEnd struct {
	ID    int    `json:"id"`
	Error string `json:"error,omitempty"`
}

func init() {
	application.RegisterEvent[SpeechProgress]("speech:progress")
	application.RegisterEvent[SpeechChunk]("speech:chunk")
	application.RegisterEvent[SpeechStarted]("speech:started")
	application.RegisterEvent[SpeechEnd]("speech:done")
}

// Status describes whether OpenVox is reachable and which voice will be used.
type Status struct {
	Reachable bool           `json:"reachable"`
	Error     string         `json:"error,omitempty"`
	Settings  store.Settings `json:"settings"`
	// AverageSeconds is the model's mean response time over its recent
	// requests, or nil if none have been recorded.
	AverageSeconds *float64 `json:"averageSeconds"`
}

// SpeechService is bound to the frontend.
type SpeechService struct {
	client   *openvox.Client
	settings *store.SettingsStore
	recent   *store.RecentStore
	timings  *store.TimingStore

	app    *application.App
	window *application.WebviewWindow
	panel  *Panel

	mu       sync.Mutex
	cancel   context.CancelFunc
	speechID int
	loaded   map[string]bool
}

func NewSpeechService(dataDir string) *SpeechService {
	ss := store.NewSettingsStore(dataDir)
	s, _ := ss.Load()
	return &SpeechService{
		client:   openvox.New(s.BaseURL),
		settings: ss,
		recent:   store.NewRecentStore(dataDir),
		timings:  store.NewTimingStore(dataDir),
		loaded:   map[string]bool{},
	}
}

func (v *SpeechService) attach(app *application.App, window *application.WebviewWindow, panel *Panel) {
	v.app, v.window, v.panel = app, window, panel
}

func (v *SpeechService) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	if s, _ := v.settings.Load(); !s.UsesOpenVox() {
		return nil
	}
	// Resolve defaults and warm the model without blocking startup.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		s, err := v.resolveSettings(ctx)
		if err != nil {
			log.Printf("openvox unavailable at startup: %v", err)
			return
		}
		if err := v.ensureLoaded(ctx, s.Model); err != nil {
			log.Printf("preloading %s: %v", s.Model, err)
		}
	}()
	return nil
}

// Status returns the effective settings and, for OpenVox, whether it's reachable.
func (v *SpeechService) Status() Status {
	if saved, err := v.settings.Load(); err == nil && !saved.UsesOpenVox() {
		return Status{Reachable: true, Settings: saved, AverageSeconds: v.averageSeconds(systemTimingKey)}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s, err := v.resolveSettings(ctx)
	if err != nil {
		saved, _ := v.settings.Load()
		return Status{Reachable: false, Error: err.Error(), Settings: saved, AverageSeconds: v.averageSeconds(saved.Model)}
	}
	return Status{Reachable: true, Settings: s, AverageSeconds: v.averageSeconds(s.Model)}
}

func (v *SpeechService) averageSeconds(model string) *float64 {
	avg, ok, err := v.timings.Average(model)
	if err != nil {
		log.Printf("loading response times: %v", err)
	}
	if !ok {
		return nil
	}
	secs := avg.Seconds()
	return &secs
}

func (v *SpeechService) ServerURL() string { return v.client.BaseURL }

func (v *SpeechService) ListModels() ([]openvox.Option, error) {
	return v.client.Models(context.Background())
}

func (v *SpeechService) ListLanguages(model string) ([]openvox.Option, error) {
	return v.client.Languages(context.Background(), model)
}

func (v *SpeechService) ListVoices(model, language string) ([]openvox.Option, error) {
	return v.client.Voices(context.Background(), model, language)
}

func (v *SpeechService) GetSettings() (store.Settings, error) {
	return v.settings.Load()
}

// SaveSettings stores the selection and warms the model if it changed.
func (v *SpeechService) SaveSettings(s store.Settings) error {
	old, _ := v.settings.Load()
	s.BaseURL = old.BaseURL
	if s.Engine == "" {
		s.Engine = old.Engine
	}
	if s.FontSize == 0 {
		s.FontSize = old.FontSize
	}
	if err := v.settings.Save(s); err != nil {
		return err
	}
	if s.UsesOpenVox() && s.Model != "" && s.Model != old.Model {
		go func() {
			if err := v.ensureLoaded(context.Background(), s.Model); err != nil {
				log.Printf("loading %s: %v", s.Model, err)
			}
		}()
	}
	return nil
}

func (v *SpeechService) Recent() ([]store.RecentEntry, error) {
	return v.recent.List()
}

// Speak starts speaking text and records it in the recent list. Audio arrives
// via speech:chunk events; speech:done fires at the end. Returns the utterance ID.
func (v *SpeechService) Speak(text string) (int, error) {
	if err := v.recent.Add(text); err != nil {
		log.Printf("saving recent: %v", err)
	}
	return v.startSpeech(text), nil
}

// Preview speaks a sample sentence with the current settings.
func (v *SpeechService) Preview() int {
	return v.startSpeech(previewText)
}

// StopSpeaking cancels any in-flight generation.
func (v *SpeechService) StopSpeaking() {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.cancel != nil {
		v.cancel()
		v.cancel = nil
	}
}

func (v *SpeechService) startSpeech(text string) int {
	v.mu.Lock()
	if v.cancel != nil {
		v.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	v.cancel = cancel
	v.speechID++
	id := v.speechID
	v.mu.Unlock()

	v.emitProgress(id, "sending")
	go func() {
		defer cancel()
		var err error
		if s, _ := v.settings.Load(); s.UsesOpenVox() {
			octx, ocancel := context.WithTimeout(ctx, speechTimeout)
			err = v.speak(octx, id, text)
			ocancel()
		} else {
			err = v.speakSystem(ctx, id, text)
		}
		end := SpeechEnd{ID: id}
		switch {
		case errors.Is(err, errStartTimeout):
			end.Error = err.Error() + "."
		case errors.Is(err, context.DeadlineExceeded):
			end.Error = fmt.Sprintf("OpenVox didn't finish within %v.", speechTimeout)
		case err != nil && !errors.Is(err, context.Canceled):
			end.Error = err.Error()
		}
		if end.Error != "" {
			log.Printf("speech %d: %v", id, err)
		}
		v.app.Event.Emit("speech:done", end)
	}()
	return id
}

// speakSystem speaks with the macOS System Voice, which plays directly rather
// than streaming audio to the frontend.
func (v *SpeechService) speakSystem(ctx context.Context, id int, text string) error {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	timer := time.AfterFunc(speechTimeout, func() { cancel(errStartTimeout) })
	defer timer.Stop()

	v.emitProgress(id, "waiting") // nothing to send over a network; it's all generation
	sentAt := time.Now()
	err := speakWithSystemVoice(ctx, id, text, func() {
		timer.Stop()
		if err := v.timings.Record(systemTimingKey, time.Since(sentAt)); err != nil {
			log.Printf("saving response time: %v", err)
		}
		v.app.Event.Emit("speech:started", SpeechStarted{ID: id})
	})
	if cause := context.Cause(ctx); errors.Is(cause, errStartTimeout) {
		return cause
	}
	return err
}

// OpenSpokenContentSettings opens System Settings where the System Voice is chosen.
func (v *SpeechService) OpenSpokenContentSettings() error {
	return exec.Command("open", "x-apple.systempreferences:com.apple.Accessibility-Settings.extension?SpokenContent").Run()
}

func (v *SpeechService) emitProgress(id int, stage string) {
	v.app.Event.Emit("speech:progress", SpeechProgress{ID: id, Stage: stage, TimeoutMs: int(speechTimeout.Milliseconds())})
}

func (v *SpeechService) speak(ctx context.Context, id int, text string) error {
	s, err := v.resolveSettings(ctx)
	if err != nil {
		return err
	}
	if err := v.ensureLoaded(ctx, s.Model); err != nil {
		return err
	}
	// Time from sending the request to the first audio, for the model's average.
	var sentAt time.Time
	gotAudio := false
	emit := func(audio []byte) {
		if !gotAudio {
			gotAudio = true
			if err := v.timings.Record(s.Model, time.Since(sentAt)); err != nil {
				log.Printf("saving response time: %v", err)
			}
		}
		v.app.Event.Emit("speech:chunk", SpeechChunk{ID: id, Audio: base64.StdEncoding.EncodeToString(audio)})
	}
	req := openvox.SpeechRequest{
		Model: s.Model, Input: text, Language: s.Language, Voice: s.Voice,
		ResponseFormat: "wav", Stream: true,
	}
	accepted := func() { v.emitProgress(id, "waiting") }
	sentAt = time.Now()
	err = v.client.SpeakWithProgress(ctx, req, accepted, emit)
	if !errors.Is(err, openvox.ErrVoiceNotFound) {
		return err
	}

	// The saved voice disappeared: pick a valid replacement and retry once.
	voices, verr := v.client.Voices(ctx, s.Model, s.Language)
	if verr != nil || len(voices) == 0 {
		return err
	}
	s.Voice = voices[0].ID
	if serr := v.settings.Save(s); serr != nil {
		log.Printf("saving replacement voice: %v", serr)
	}
	req.Voice = s.Voice
	sentAt = time.Now()
	return v.client.SpeakWithProgress(ctx, req, accepted, emit)
}

// resolveSettings fills in any missing model/language/voice with the first
// option the server offers, persisting the result.
func (v *SpeechService) resolveSettings(ctx context.Context) (store.Settings, error) {
	s, err := v.settings.Load()
	if err != nil {
		return s, err
	}
	orig := s

	models, err := v.client.Models(ctx)
	if err != nil {
		return s, err
	}
	if len(models) == 0 {
		return s, errors.New("OpenVox has no models available")
	}
	if !containsID(models, s.Model) {
		s.Model, s.Language, s.Voice = models[0].ID, "", ""
	}

	if s.Language == "" {
		// The languages endpoint may not exist for every model; language is optional.
		if langs, err := v.client.Languages(ctx, s.Model); err == nil && len(langs) > 0 {
			s.Language = langs[0].ID
			if containsID(langs, systemLanguage()) {
				s.Language = systemLanguage()
			}
		}
	}

	if s.Voice == "" {
		voices, err := v.client.Voices(ctx, s.Model, s.Language)
		if err != nil {
			return s, err
		}
		if len(voices) > 0 {
			s.Voice = voices[0].ID
		}
	}

	if s != orig {
		if err := v.settings.Save(s); err != nil {
			return s, err
		}
	}
	return s, nil
}

func (v *SpeechService) ensureLoaded(ctx context.Context, model string) error {
	v.mu.Lock()
	done := v.loaded[model]
	v.mu.Unlock()
	if done {
		return nil
	}
	if err := v.client.Load(ctx, model); err != nil {
		return err
	}
	v.mu.Lock()
	v.loaded[model] = true
	v.mu.Unlock()
	return nil
}

// systemLanguage returns the macOS locale's language code, e.g. "en" for en_US.
var systemLanguage = sync.OnceValue(func() string {
	locale := os.Getenv("LANG")
	if out, err := exec.Command("defaults", "read", "-g", "AppleLocale").Output(); err == nil {
		locale = strings.TrimSpace(string(out))
	}
	lang, _, _ := strings.Cut(locale, "_")
	lang, _, _ = strings.Cut(lang, "-")
	lang, _, _ = strings.Cut(lang, ".")
	return strings.ToLower(lang)
})

func containsID(opts []openvox.Option, id string) bool {
	return id != "" && slices.ContainsFunc(opts, func(o openvox.Option) bool { return o.ID == id })
}

// --- window helpers ---

// ScreenHeight returns the height of the screen the panel is on, in points.
func (v *SpeechService) ScreenHeight() int {
	if scr, err := v.window.GetScreen(); err == nil && scr != nil && scr.Size.Height > 0 {
		return scr.Size.Height
	}
	if scr := v.app.Screen.GetPrimary(); scr != nil {
		return scr.Size.Height
	}
	return 900
}

// SetPanelSize resizes the panel to fit its content, keeping its top-left corner in place.
func (v *SpeechService) SetPanelSize(width, height int) {
	maxH := v.ScreenHeight() * 9 / 10
	v.panel.SetSize(max(200, width), max(120, min(height, maxH)))
}

func (v *SpeechService) HidePanel() {
	v.panel.Hide()
}

func (v *SpeechService) Quit() {
	v.app.Quit()
}
