// Package store persists LivingSpeech settings and recent phrases as JSON files.
package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

// Settings holds the user's voice selection.
type Settings struct {
	Model    string `json:"model"`
	Language string `json:"language"`
	Voice    string `json:"voice"`
	BaseURL  string `json:"baseURL,omitempty"`
}

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
