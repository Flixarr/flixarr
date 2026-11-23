package server

import (
	"testing"

	"github.com/Flixarr/flixarr/internal/store"
)

func TestMatchTitles(t *testing.T) {
	all := []store.PlexTitle{{Type: "series", TmdbID: 1396, Title: "Breaking Bad"}, {Type: "movie", TmdbID: 1367, Title: "Rocky II"}, {Type: "movie", TmdbID: 1366, Title: "Rocky"},
		{Type: "series", TmdbID: 2316, Title: "The Office"}, {Type: "movie", TmdbID: 1542, Title: "Office Space"}, {Type: "movie", TmdbID: 194, Title: "Amélie"}, {Type: "movie", TmdbID: 13804, Title: "Fast & Furious"},
		{Type: "movie", TmdbID: 50619, Title: "The Twilight Saga: Breaking Dawn - Part 1"}, {Type: "movie", TmdbID: 299534, Title: "Avengers: Endgame"}}
	for _, c := range []struct {
		q    string
		want int
	}{{"brkng bad", 1396}, {"breaking", 1396}, {"rocky 2", 1367}, {"office", 2316}, {"amelie", 194}, {"fast and furious", 13804},
		{"brekaing bad", 1396}, {"brekai", 1396}, {"breka", 1396}, {"brekaing", 1396}, {"braeking bad", 1396}, {"offcie", 2316},
		{"breaking dawn", 50619}, {"braeking dawn", 50619}, {"endgame", 299534}, {"engdame", 299534}} {
		got := matchTitles(c.q, all, 5)
		if len(got) == 0 || got[0].TmdbID != c.want {
			t.Errorf("%q: got %v, want %d first", c.q, got, c.want)
		}
	}
	// A different first letter isn't a typo of the title.
	if got := matchTitles("rbeaking", all, 5); len(got) != 0 {
		t.Errorf("rbeaking matched %v", got)
	}
	// A first-word match ranks above the same match on a later word.
	if got := matchTitles("brek", all, 5); len(got) < 2 || got[0].TmdbID != 1396 || got[1].TmdbID != 50619 {
		t.Errorf("brek: got %v, want Breaking Bad then Breaking Dawn", got)
	}
	if got := matchTitles("zzz", all, 5); len(got) != 0 {
		t.Errorf("zzz matched %v", got)
	}
}
