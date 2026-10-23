package jobs

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/Flixarr/flixarr/internal/secret"
	"github.com/Flixarr/flixarr/internal/store"
)

// Spread jobs (#165): each install runs them at its own offset, the same every day, within the spread;
// jobs without a spread run on time.
func TestOffset(t *testing.T) {
	dir := t.TempDir()
	box, _ := secret.Load(dir)
	st, _ := store.Open(dir, box)
	defer st.Close()
	r := &Runner{Store: st, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	a := &Job{ID: "plex-episodes", Spread: 2 * time.Hour}
	b := &Job{ID: "watchlist", Spread: 10 * time.Minute}
	if off := r.offset(&Job{ID: "backup"}); off != 0 {
		t.Fatalf("no spread: %v", off)
	}
	oa, ob := r.offset(a), r.offset(b)
	if oa < 0 || oa >= a.Spread || ob < 0 || ob >= b.Spread {
		t.Fatalf("out of range: %v %v", oa, ob)
	}
	if r.offset(a) != oa {
		t.Fatal("offset changed between calls")
	}
	// Another install (another seed) lands somewhere else.
	st.SetSetting(keySeed, "another install")
	if r.offset(a) == oa && r.offset(b) == ob {
		t.Fatal("two installs with the same offsets")
	}
}
