package store

import (
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	RecentWindow = 24 * time.Hour
	recentMax    = 50
)

// RecentEntry is a phrase that was spoken.
type RecentEntry struct {
	Text string    `json:"text"`
	At   time.Time `json:"at"`
}

// RecentStore keeps phrases spoken within the last 24 hours, newest first.
type RecentStore struct {
	path    string
	mu      sync.Mutex
	entries []RecentEntry
	loaded  bool
	Now     func() time.Time
}

func NewRecentStore(dir string) *RecentStore {
	return &RecentStore{path: filepath.Join(dir, "recent.json"), Now: time.Now}
}

// List returns non-expired entries, newest first.
func (r *RecentStore) List() ([]RecentEntry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.load(); err != nil {
		return nil, err
	}
	if r.prune() {
		if err := writeJSON(r.path, r.entries); err != nil {
			return nil, err
		}
	}
	return append([]RecentEntry(nil), r.entries...), nil
}

// Add records text, moving an existing identical entry to the top.
func (r *RecentStore) Add(text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.load(); err != nil {
		return err
	}
	kept := r.entries[:0]
	for _, e := range r.entries {
		if e.Text != text {
			kept = append(kept, e)
		}
	}
	r.entries = append([]RecentEntry{{Text: text, At: r.Now()}}, kept...)
	r.prune()
	return writeJSON(r.path, r.entries)
}

func (r *RecentStore) load() error {
	if r.loaded {
		return nil
	}
	if err := readJSON(r.path, &r.entries); err != nil {
		return err
	}
	r.loaded = true
	return nil
}

// prune drops expired entries and caps the list. Reports whether anything changed.
func (r *RecentStore) prune() bool {
	cutoff := r.Now().Add(-RecentWindow)
	before := len(r.entries)
	kept := r.entries[:0]
	for _, e := range r.entries {
		if e.At.After(cutoff) {
			kept = append(kept, e)
		}
	}
	if len(kept) > recentMax {
		kept = kept[:recentMax]
	}
	r.entries = kept
	return len(kept) != before
}
