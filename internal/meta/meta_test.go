package meta

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
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
		{KindPeople, 0, 7 * 24 * time.Hour},
		{KindLists, 0, 24 * time.Hour},
		{KindSearches, 0, 7 * 24 * time.Hour},
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
	for _, p := range []string{"movie/603", "movie/603", "series/1399", "series/1399/season/1", "series/1399/season/1", "collection/86311", "person/1", "lists/trending", "lists/trending", "lists/trending"} {
		if _, _, err := c.Get(ctx, p, url.Values{}); err != nil {
			t.Fatalf("%s: %v", p, err)
		}
	}
	got := c.Counters()
	want := map[string]HitMiss{KindMovies: {1, 1}, KindSeries: {0, 1}, KindSeasons: {1, 1}, KindCollections: {0, 1}, KindPeople: {0, 1}, KindLists: {2, 1}, KindSearches: {0, 0}}
	for k, w := range want {
		if got.ByKind[k] != w {
			t.Errorf("%s: %+v, want %+v", k, got.ByKind[k], w)
		}
	}
	if got.Hits != 4 || got.Misses != 6 {
		t.Errorf("totals %d hits %d misses", got.Hits, got.Misses)
	}
}

// Every path the API allows is sorted into a kind with its own counter and Storage row.
func TestKindOf(t *testing.T) {
	for p, want := range map[string]string{
		"movie/603": KindMovies, "movie/603/recommendations": KindMovies, "series/1399": KindSeries, "series/1399/ids": KindSeries,
		"series/1399/season/2": KindSeasons, "collection/86311": KindCollections, "person/1": KindPeople, "credit/651dd86d74507d00e2105e2c": KindPeople,
		"lists/trending": KindLists, "discover/movie": KindLists, "genres/movie": KindLists, "contributors": KindLists, "search": KindSearches,
	} {
		if !allowed(p) {
			t.Fatalf("%s refused", p)
		}
		if got := kindOf(p); got != want {
			t.Errorf("%s: %s, want %s", p, got, want)
		}
		if !slices.Contains(Kinds, kindOf(p)) {
			t.Errorf("%s: kind %s has no counter", p, kindOf(p))
		}
	}
}

// Searches always go to the API and are never saved (#244).
func TestSearchesNotSaved(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Cache-Control", "s-maxage=3600")
		w.Write([]byte(`{"results":[]}`))
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
	q := url.Values{"q": {"dune"}}
	for range 2 {
		if _, _, err := c.Get(context.Background(), "search", q); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 2 {
		t.Errorf("%d API calls for 2 searches, want 2", calls)
	}
	kinds, _ := st.CacheStats()
	for _, k := range kinds {
		if k.Kind == KindSearches {
			t.Errorf("%d searches saved", k.Count)
		}
	}
}

// Background calls (#165) take turns BackgroundEvery apart and say so in a header; others don't wait.
func TestBackgroundPace(t *testing.T) {
	var marked int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Flixarr-Background") == "1" {
			marked++
		}
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	dir := t.TempDir()
	box, _ := secret.Load(dir)
	st, _ := store.Open(dir, box)
	defer st.Close()
	c := New(srv.URL, "test", st)
	every := BackgroundEvery
	BackgroundEvery = 100 * time.Millisecond
	t.Cleanup(func() { BackgroundEvery = every })
	bg := Background(context.Background())
	start := time.Now()
	for _, id := range []string{"1", "2", "3"} {
		c.Get(bg, "movie/"+id, url.Values{})
	}
	if took := time.Since(start); took < 200*time.Millisecond {
		t.Fatalf("3 background calls in %v", took)
	}
	if marked != 3 {
		t.Fatalf("marked %d", marked)
	}
	start = time.Now()
	c.Get(context.Background(), "movie/4", url.Values{})
	if took := time.Since(start); took > 90*time.Millisecond {
		t.Fatalf("a page waited %v", took)
	}
}
