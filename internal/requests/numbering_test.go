package requests

import (
	"encoding/json"
	"slices"
	"testing"
)

func TestSameTitle(t *testing.T) {
	for _, c := range [][2]string{
		{"Goodbye, Toby (1)", "Goodbye Toby"},
		{"The Chair Model", "Chair Model"},
		{"Do You Want to Build A...Music Video?", "Do You Want to Build a... Music Video?"},
		{"Niagara, Part 2", "Niagara"},
		{"Life Is Going To Be Gouda", "Life is Going to Be Gouda"},
	} {
		if sameTitle(c[0]) != sameTitle(c[1]) {
			t.Errorf("%q and %q should match: %q vs %q", c[0], c[1], sameTitle(c[0]), sameTitle(c[1]))
		}
	}
	if sameTitle("Fun Run") == sameTitle("Money") {
		t.Error("different titles match")
	}
}

// 1000-lb Sisters season 6: TMDB has Clapback as episode 11, Sonarr (TheTVDB) as a special.
func TestSpecialsIn(t *testing.T) {
	tmdb := []string{"Life is Going to Be Gouda", "Recipe for Disaster", "War and Peas", "I Candy-cane't", "Carbe Diem",
		"Limo Beans", "Turning a New Beef", "Peas Out, Yo!", "Bangers and Clash", "Nothing is Impastable", "Clapback"}
	sonarr := []string{"Life Is Going To Be Gouda", "Recipe for Disaster", "War and Peas", "I Candy-cane't", "Carbe Diem",
		"Limo Beans", "Turning a New Beef", "Peas Out, Yo!", "Bangers and Clash", "Nothing is Impastable"}
	specials := map[int]string{501: "Sister Act", 502: "Clapback", 503: "Tell All"}
	if got := specialsIn(tmdb, sonarr, specials); !slices.Equal(got, []int{502}) {
		t.Fatalf("got %v, want [502]", got)
	}
	// The Office season 4: TheTVDB's two parters are the same episodes, not specials.
	tmdb = []string{"Fun Run", "Dunder Mifflin Infinity", "Chair Model", "Goodbye Toby"}
	sonarr = []string{"Fun Run (1)", "Fun Run (2)", "Dunder Mifflin Infinity (1)", "Dunder Mifflin Infinity (2)", "The Chair Model", "Goodbye, Toby (1)", "Goodbye, Toby (2)"}
	specials = map[int]string{601: "Fun Run", 602: "The Accountants"}
	if got := specialsIn(tmdb, sonarr, specials); len(got) != 0 {
		t.Fatalf("got %v, want none", got)
	}
}

func TestSeasonCounts(t *testing.T) {
	var sr map[string]any
	json.Unmarshal([]byte(`{"seasons":[
		{"seasonNumber":0,"statistics":{"totalEpisodeCount":21,"episodeCount":0}},
		{"seasonNumber":4,"statistics":{"totalEpisodeCount":19,"episodeCount":0,"previousAiring":"2008-05-16T00:00:00Z"}},
		{"seasonNumber":9,"statistics":{"totalEpisodeCount":11,"episodeCount":9,"previousAiring":"2026-10-28T01:00:00Z","nextAiring":"2026-11-04T02:00:00Z"}},
		{"seasonNumber":10,"statistics":{"totalEpisodeCount":8,"episodeCount":0}}]}`), &sr)
	got := seasonCounts(sr)
	if _, ok := got[0]; ok {
		t.Error("specials counted")
	}
	// Ended (even unmonitored): all of it. Airing: what's out. Not started: none.
	if got[4].Aired != 19 || got[9].Aired != 9 || got[9].Total != 11 || got[10].Aired != 0 || got[10].Total != 8 {
		t.Fatalf("got %+v", got)
	}
}
