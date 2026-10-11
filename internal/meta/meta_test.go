package meta

import (
	"testing"
	"time"
)

func TestFreshFor(t *testing.T) {
	for cc, want := range map[string]time.Duration{
		"public, max-age=300, s-maxage=21600": 6 * time.Hour,
		"s-maxage=604800":                     7 * 24 * time.Hour,
		"public, max-age=300":                 0,
		"no-store":                            0,
		"":                                    0,
	} {
		if got := freshFor(cc); got != want {
			t.Errorf("freshFor(%q) = %v, want %v", cc, got, want)
		}
	}
}

func TestKeepAutomatic(t *testing.T) {
	auto := Config{PagesMin: Auto, ListsMin: 180, SearchesMin: 60}
	if got := auto.keep(KindTitles, 6*time.Hour); got != 6*time.Hour {
		t.Errorf("automatic title = %v, want the API's 6h", got)
	}
	if got := auto.keep(KindPeople, 0); got != 24*time.Hour {
		t.Errorf("automatic with no API time = %v, want 1 day", got)
	}
	if got := auto.keep(KindLists, 6*time.Hour); got != 3*time.Hour {
		t.Errorf("lists = %v, want the setting's 3h", got)
	}
	fixed := Config{PagesMin: 1440}
	if got := fixed.keep(KindTitles, 6*time.Hour); got != 24*time.Hour {
		t.Errorf("fixed title = %v, want 1 day", got)
	}
}
