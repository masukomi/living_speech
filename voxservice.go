package main

import (
	"context"
	"encoding/base64"
	"errors"
	"log"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"voxbox/internal/openvox"
	"voxbox/internal/store"
)

const previewText = "Hello! This is how I sound."

// Event payloads sent to the frontend. ID identifies the utterance so the
// frontend can ignore chunks from a request it has already stopped.
type SpeechChunk struct {
	ID    int    `json:"id"`
	Audio string `json:"audio"` // base64 WAV
}

type SpeechEnd struct {
	ID    int    `json:"id"`
	Error string `json:"error,omitempty"`
}

func init() {
	application.RegisterEvent[SpeechChunk]("speech:chunk")
	application.RegisterEvent[SpeechEnd]("speech:done")
}

// Status describes whether OpenVox is reachable and which voice will be used.
type Status struct {
	Reachable bool           `json:"reachable"`
	Error     string         `json:"error,omitempty"`
	Settings  store.Settings `json:"settings"`
}

// VoxService is bound to the frontend.
type VoxService struct {
	client   *openvox.Client
	settings *store.SettingsStore
	recent   *store.RecentStore

	app    *application.App
	window *application.WebviewWindow
	tray   *application.SystemTray

	mu       sync.Mutex
	cancel   context.CancelFunc
	speechID int
	loaded   map[string]bool
}

func NewVoxService(dataDir string) *VoxService {
	ss := store.NewSettingsStore(dataDir)
	s, _ := ss.Load()
	return &VoxService{
		client:   openvox.New(s.BaseURL),
		settings: ss,
		recent:   store.NewRecentStore(dataDir),
		loaded:   map[string]bool{},
	}
}

func (v *VoxService) attach(app *application.App, window *application.WebviewWindow, tray *application.SystemTray) {
	v.app, v.window, v.tray = app, window, tray
}

func (v *VoxService) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
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

// Status checks the server and returns the effective settings.
func (v *VoxService) Status() Status {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s, err := v.resolveSettings(ctx)
	if err != nil {
		saved, _ := v.settings.Load()
		return Status{Reachable: false, Error: err.Error(), Settings: saved}
	}
	return Status{Reachable: true, Settings: s}
}

func (v *VoxService) ServerURL() string { return v.client.BaseURL }

func (v *VoxService) ListModels() ([]openvox.Option, error) {
	return v.client.Models(context.Background())
}

func (v *VoxService) ListLanguages(model string) ([]openvox.Option, error) {
	return v.client.Languages(context.Background(), model)
}

func (v *VoxService) ListVoices(model, language string) ([]openvox.Option, error) {
	return v.client.Voices(context.Background(), model, language)
}

func (v *VoxService) GetSettings() (store.Settings, error) {
	return v.settings.Load()
}

// SaveSettings stores the selection and warms the model if it changed.
func (v *VoxService) SaveSettings(s store.Settings) error {
	old, _ := v.settings.Load()
	s.BaseURL = old.BaseURL
	if err := v.settings.Save(s); err != nil {
		return err
	}
	if s.Model != "" && s.Model != old.Model {
		go func() {
			if err := v.ensureLoaded(context.Background(), s.Model); err != nil {
				log.Printf("loading %s: %v", s.Model, err)
			}
		}()
	}
	return nil
}

func (v *VoxService) Recent() ([]store.RecentEntry, error) {
	return v.recent.List()
}

// Speak starts speaking text and records it in the recent list. Audio arrives
// via speech:chunk events; speech:done fires at the end. Returns the utterance ID.
func (v *VoxService) Speak(text string) (int, error) {
	if err := v.recent.Add(text); err != nil {
		log.Printf("saving recent: %v", err)
	}
	return v.startSpeech(text), nil
}

// Preview speaks a sample sentence with the current settings.
func (v *VoxService) Preview() int {
	return v.startSpeech(previewText)
}

// StopSpeaking cancels any in-flight generation.
func (v *VoxService) StopSpeaking() {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.cancel != nil {
		v.cancel()
		v.cancel = nil
	}
}

func (v *VoxService) startSpeech(text string) int {
	v.mu.Lock()
	if v.cancel != nil {
		v.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	v.cancel = cancel
	v.speechID++
	id := v.speechID
	v.mu.Unlock()

	go func() {
		defer cancel()
		err := v.speak(ctx, id, text)
		end := SpeechEnd{ID: id}
		if err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("speech %d: %v", id, err)
			end.Error = err.Error()
		}
		v.app.Event.Emit("speech:done", end)
	}()
	return id
}

func (v *VoxService) speak(ctx context.Context, id int, text string) error {
	s, err := v.resolveSettings(ctx)
	if err != nil {
		return err
	}
	if err := v.ensureLoaded(ctx, s.Model); err != nil {
		return err
	}
	emit := func(audio []byte) {
		v.app.Event.Emit("speech:chunk", SpeechChunk{ID: id, Audio: base64.StdEncoding.EncodeToString(audio)})
	}
	req := openvox.SpeechRequest{
		Model: s.Model, Input: text, Language: s.Language, Voice: s.Voice,
		ResponseFormat: "wav", Stream: true,
	}
	err = v.client.Speak(ctx, req, emit)
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
	return v.client.Speak(ctx, req, emit)
}

// resolveSettings fills in any missing model/language/voice with the first
// option the server offers, persisting the result.
func (v *VoxService) resolveSettings(ctx context.Context) (store.Settings, error) {
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

func (v *VoxService) ensureLoaded(ctx context.Context, model string) error {
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
func (v *VoxService) ScreenHeight() int {
	if scr, err := v.window.GetScreen(); err == nil && scr != nil && scr.Size.Height > 0 {
		return scr.Size.Height
	}
	if scr := v.app.Screen.GetPrimary(); scr != nil {
		return scr.Size.Height
	}
	return 900
}

// SetPanelHeight resizes the panel to fit its content, keeping it pinned under the menu bar.
func (v *VoxService) SetPanelHeight(height int) {
	maxH := v.ScreenHeight() * 9 / 10
	height = max(120, min(height, maxH))
	w, h := v.window.Size()
	if h == height {
		return
	}
	v.window.SetSize(w, height)
	if v.window.IsVisible() {
		_ = v.tray.PositionWindow(v.window, panelOffset)
	}
}

func (v *VoxService) HidePanel() {
	v.window.Hide()
}

func (v *VoxService) Quit() {
	v.app.Quit()
}
