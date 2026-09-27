package store

import "testing"

func TestWindowStoreRoundTrip(t *testing.T) {
	ws := NewWindowStore(t.TempDir())
	pos, err := ws.Load()
	if err != nil || pos != nil {
		t.Fatalf("empty store: pos=%v err=%v", pos, err)
	}
	if err := ws.Save(WindowPosition{X: 1200, Y: -40}); err != nil {
		t.Fatal(err)
	}
	pos, err = ws.Load()
	if err != nil || pos == nil || pos.X != 1200 || pos.Y != -40 {
		t.Fatalf("got pos=%v err=%v", pos, err)
	}
}
