package store

import (
	"path/filepath"
	"sync"
	"time"
)

// timingsMax is how many recent response times are kept per model.
const timingsMax = 100

// TimingStore records how long each model took to respond, keeping the most
// recent timingsMax samples per model.
type TimingStore struct {
	path    string
	mu      sync.Mutex
	samples map[string][]int64 // model -> response times in milliseconds, oldest first
	loaded  bool
}

func NewTimingStore(dir string) *TimingStore {
	return &TimingStore{path: filepath.Join(dir, "timings.json")}
}

// Record adds a response time for model, dropping the oldest beyond the cap.
func (t *TimingStore) Record(model string, d time.Duration) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if err := t.load(); err != nil {
		return err
	}
	s := append(t.samples[model], d.Milliseconds())
	if len(s) > timingsMax {
		s = s[len(s)-timingsMax:]
	}
	t.samples[model] = s
	return writeJSON(t.path, t.samples)
}

// Average returns the mean response time for model, and false if there's no data.
func (t *TimingStore) Average(model string) (time.Duration, bool, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if err := t.load(); err != nil {
		return 0, false, err
	}
	s := t.samples[model]
	if len(s) == 0 {
		return 0, false, nil
	}
	var sum int64
	for _, ms := range s {
		sum += ms
	}
	return time.Duration(sum/int64(len(s))) * time.Millisecond, true, nil
}

func (t *TimingStore) load() error {
	if t.loaded {
		return nil
	}
	if err := readJSON(t.path, &t.samples); err != nil {
		return err
	}
	if t.samples == nil {
		t.samples = map[string][]int64{}
	}
	t.loaded = true
	return nil
}
