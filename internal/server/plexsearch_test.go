package server

import (
	"testing"

	"github.com/Flixarr/flixarr/internal/store"
)

func TestMatchTitles(t *testing.T) {
	all := []store.PlexTitle{{Type: "series", TmdbID: 1396, Title: "Breaking Bad"}, {Type: "movie", TmdbID: 1367, Title: "Rocky II"}, {Type: "movie", TmdbID: 1366, Title: "Rocky"},
		{Type: "series", TmdbID: 2316, Title: "The Office"}, {Type: "movie", TmdbID: 1542, Title: "Office Space"}, {Type: "movie", TmdbID: 194, Title: "Amélie"}, {Type: "movie", TmdbID: 13804, Title: "Fast & Furious"}}
	for _, c := range []struct {
		q    string
		want int
	}{{"brkng bad", 1396}, {"breaking", 1396}, {"rocky 2", 1367}, {"office", 2316}, {"amelie", 194}, {"fast and furious", 13804}} {
		got := matchTitles(c.q, all, 5)
		if len(got) == 0 || got[0].TmdbID != c.want {
			t.Errorf("%q: got %v, want %d first", c.q, got, c.want)
		}
	}
	if got := matchTitles("zzz", all, 5); len(got) != 0 {
		t.Errorf("zzz matched %v", got)
	}
}
