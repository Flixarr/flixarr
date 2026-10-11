package meta

import (
	"testing"
	"time"
)

func TestFreshFor(t *testing.T) {
	for _, c := range []struct {
		cc, age string
		want    time.Duration
	}{
		{"public, max-age=300, s-maxage=21600", "", 6 * time.Hour},
		{"public, max-age=300, s-maxage=21600", "3600", 5 * time.Hour},
		{"s-maxage=3600", "9999", time.Second},
		{"s-maxage=604800", "junk", 7 * 24 * time.Hour},
		{"public, max-age=300", "100", 0},
		{"no-store", "", 0},
		{"", "", 0},
	} {
		if got := freshFor(c.cc, c.age); got != c.want {
			t.Errorf("freshFor(%q, %q) = %v, want %v", c.cc, c.age, got, c.want)
		}
	}
}

func TestKeepAutomatic(t *testing.T) {
	auto := Config{Auto: true, PagesMin: 1440, ListsMin: 180, SearchesMin: 60}
	for _, c := range []struct {
		kind  string
		fresh time.Duration
		want  time.Duration
	}{
		{KindTitles, 6 * time.Hour, 6 * time.Hour},
		{KindPeople, 0, 24 * time.Hour},
		{KindLists, 0, time.Hour},
		{KindSearches, 0, time.Hour},
		{KindLists, 40 * time.Minute, 40 * time.Minute},
		{KindTitles, 20 * time.Second, minKeep},
	} {
		if got := auto.keep(c.kind, c.fresh); got != c.want {
			t.Errorf("automatic %s with %v = %v, want %v", c.kind, c.fresh, got, c.want)
		}
	}
	fixed := Config{Auto: false, PagesMin: 1440, ListsMin: 180, SearchesMin: 60}
	if got := fixed.keep(KindTitles, 6*time.Hour); got != 24*time.Hour {
		t.Errorf("fixed title = %v, want 1 day", got)
	}
	if got := fixed.keep(KindLists, time.Hour); got != 3*time.Hour {
		t.Errorf("fixed lists = %v, want 3h", got)
	}
}
