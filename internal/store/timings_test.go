package store

import (
	"testing"
	"time"
)

func TestTimingStoreAverageAndCap(t *testing.T) {
	dir := t.TempDir()
	ts := NewTimingStore(dir)

	if _, ok, err := ts.Average("kokoro"); err != nil || ok {
		t.Fatalf("empty: ok=%v err=%v", ok, err)
	}

	// 10 slow samples that should be pushed out, then 100 at 2s.
	for range 10 {
		if err := ts.Record("kokoro", 60*time.Second); err != nil {
			t.Fatal(err)
		}
	}
	for range timingsMax {
		if err := ts.Record("kokoro", 2*time.Second); err != nil {
			t.Fatal(err)
		}
	}
	if err := ts.Record("qwen", 30*time.Second); err != nil {
		t.Fatal(err)
	}

	reloaded := NewTimingStore(dir)
	avg, ok, err := reloaded.Average("kokoro")
	if err != nil || !ok || avg != 2*time.Second {
		t.Fatalf("kokoro avg=%v ok=%v err=%v", avg, ok, err)
	}
	if n := len(reloaded.samples["kokoro"]); n != timingsMax {
		t.Fatalf("kept %d samples, want %d", n, timingsMax)
	}
	if avg, ok, _ := reloaded.Average("qwen"); !ok || avg != 30*time.Second {
		t.Fatalf("qwen avg=%v ok=%v", avg, ok)
	}
}
