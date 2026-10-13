package store

import (
	"testing"

	"github.com/Flixarr/flixarr/internal/secret"
)

// A slower scan writing after a newer one must not make the newer scan's prune drop the title.
func TestPlexSeenAtNeverGoesBack(t *testing.T) {
	dir := t.TempDir()
	box, _ := secret.Load(dir)
	s, err := Open(dir, box)
	if err != nil {
		t.Fatal(err)
	}
	s.SavePlexItem(PlexItem{Type: "movie", TmdbID: 603, RatingKey: "9", SeenAt: "2026-10-13T00:00:02.000000000Z"})
	s.SavePlexItem(PlexItem{Type: "movie", TmdbID: 603, RatingKey: "9", SeenAt: "2026-10-13T00:00:01.000000000Z"})
	if gone, _ := s.PrunePlexItems("2026-10-13T00:00:02.000000000Z"); len(gone) != 0 {
		t.Fatalf("pruned: %v", gone)
	}
	if gone, _ := s.RemovePlexItems([]string{"9"}); len(gone) != 1 || gone[0] != "movie:603" {
		t.Fatalf("remove by rating key: %v", gone)
	}
}
