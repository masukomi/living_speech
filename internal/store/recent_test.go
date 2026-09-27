package store

import (
	"testing"
	"time"
)

func TestRecentDedupeAndPrune(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	r := NewRecentStore(dir)
	r.Now = func() time.Time { return now }

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(r.Add("old"))
	now = now.Add(20 * time.Hour)
	must(r.Add("hello"))
	must(r.Add("  "))
	must(r.Add("world"))
	must(r.Add("hello")) // moves to top

	list, err := r.List()
	must(err)
	if got := texts(list); got != "hello,world,old" {
		t.Fatalf("got %s", got)
	}

	now = now.Add(5 * time.Hour) // "old" is now 25h old
	r2 := NewRecentStore(dir)    // reload from disk
	r2.Now = r.Now
	list, err = r2.List()
	must(err)
	if got := texts(list); got != "hello,world" {
		t.Fatalf("after prune got %s", got)
	}
}

func texts(es []RecentEntry) string {
	s := ""
	for i, e := range es {
		if i > 0 {
			s += ","
		}
		s += e.Text
	}
	return s
}
