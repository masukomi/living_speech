// Package store persists LivingSpeech settings and recent phrases as JSON files.
package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

// Speech engines.
const (
	// EngineSystem speaks with the macOS System Voice (Accessibility › Spoken Content).
	EngineSystem = "system"
	// EngineOpenVox speaks with a model from the local OpenVox server.
	EngineOpenVox = "openvox"
)

// Settings holds the user's voice selection.
type Settings struct {
	// Engine is EngineSystem or EngineOpenVox; empty means EngineSystem.
	Engine   string `json:"engine,omitempty"`
	Model    string `json:"model"`
	Language string `json:"language"`
	Voice    string `json:"voice"`
	// FontSize is the UI's base font size in points; 0 means the default.
	FontSize int `json:"fontSize,omitempty"`
	// Pronunciations holds the user's custom pronunciations, one
	// "word -> replacement" per line (see package pronounce).
	Pronunciations string `json:"pronunciations,omitempty"`
	BaseURL        string `json:"baseURL,omitempty"`
}

// UsesOpenVox reports whether speech goes through OpenVox.
func (s Settings) UsesOpenVox() bool { return s.Engine == EngineOpenVox }

// SettingsStore loads and saves Settings to a JSON file.
type SettingsStore struct {
	path string
	mu   sync.Mutex
}

func NewSettingsStore(dir string) *SettingsStore {
	return &SettingsStore{path: filepath.Join(dir, "settings.json")}
}

func (s *SettingsStore) Load() (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out Settings
	err := readJSON(s.path, &out)
	return out, err
}

func (s *SettingsStore) Save(v Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return writeJSON(s.path, v)
}

// DefaultDir is ~/Library/Application Support/LivingSpeech on macOS.
func DefaultDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "LivingSpeech"), nil
}

func readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

func writeJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
