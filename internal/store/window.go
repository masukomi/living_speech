package store

import (
	"path/filepath"
	"sync"
)

// WindowPosition is the panel's top-left corner in screen points.
type WindowPosition struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// WindowStore remembers where the panel was last placed.
type WindowStore struct {
	path string
	mu   sync.Mutex
}

func NewWindowStore(dir string) *WindowStore {
	return &WindowStore{path: filepath.Join(dir, "window.json")}
}

// Load returns the saved position, or nil if none has been saved.
func (w *WindowStore) Load() (*WindowPosition, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	var pos *WindowPosition
	err := readJSON(w.path, &pos)
	return pos, err
}

func (w *WindowStore) Save(pos WindowPosition) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return writeJSON(w.path, pos)
}
