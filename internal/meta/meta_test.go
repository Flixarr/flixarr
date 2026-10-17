package meta

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/Flixarr/flixarr/internal/secret"
	"github.com/Flixarr/flixarr/internal/store"
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

func TestAllowedPaths(t *testing.T) {
	for _, p := range []string{"credit/651dd86d74507d00e2105e2c", "contributors", "movie/603"} {
		if !allowed(p) {
			t.Fatalf("%s refused", p)
		}
	}
	if allowed("admin/secrets") {
		t.Fatal("an unknown path was let through")
	}
}

// Each Storage row counts its own hits and misses; the totals still add up.
func TestCountersByKind(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "s-maxage=3600")
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	dir := t.TempDir()
	box, _ := secret.Load(dir)
	st, err := store.Open(dir, box)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	c := New(srv.URL, "test", st)
	ctx := context.Background()
	for _, p := range []string{"movie/603", "movie/603", "person/1", "lists/trending", "lists/trending", "lists/trending"} {
		if _, _, err := c.Get(ctx, p, url.Values{}); err != nil {
			t.Fatalf("%s: %v", p, err)
		}
	}
	got := c.Counters()
	want := map[string]HitMiss{KindTitles: {1, 1}, KindPeople: {0, 1}, KindLists: {2, 1}, KindSearches: {0, 0}}
	for k, w := range want {
		if got.ByKind[k] != w {
			t.Errorf("%s: %+v, want %+v", k, got.ByKind[k], w)
		}
	}
	if got.Hits != 3 || got.Misses != 3 {
		t.Errorf("totals %d hits %d misses", got.Hits, got.Misses)
	}
}
