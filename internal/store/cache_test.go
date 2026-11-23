package store

import (
	"testing"
	"time"

	"github.com/Flixarr/flixarr/internal/secret"
)

// Titles saved before the split land in their own row (movies, series, seasons, collections) and nothing is lost.
func TestCacheTitleKindsMigration(t *testing.T) {
	dir := t.TempDir()
	box, _ := secret.Load(dir)
	s, err := Open(dir, box)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	keys := map[string]string{
		"movie/603?language=en-US&region=US":            "movies",
		"movie/603/recommendations":                     "movies",
		"series/1399?language=en-US":                    "series",
		"series/1399/ids":                               "series",
		"series/1399/season/2?language=en-US&region=US": "seasons",
		"collection/86311":                              "collections",
		"contributors":                                  "lists",
	}
	exp := time.Now().Add(time.Hour)
	for k := range keys {
		if err := s.CachePut(k, CacheEntry{Kind: "titles", Status: 200, Body: []byte(`{}`), Expires: exp}); err != nil {
			t.Fatal(err)
		}
	}
	s.CachePut("person/1", CacheEntry{Kind: "people", Status: 200, Body: []byte(`{}`), Expires: exp})
	body, err := migrations.ReadFile("migrations/0032_cache_title_kinds.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(string(body)); err != nil {
		t.Fatal(err)
	}
	for k, want := range keys {
		e, err := s.CacheGet(k)
		if err != nil {
			t.Fatalf("%s: %v", k, err)
		}
		if e.Kind != want {
			t.Errorf("%s: %s, want %s", k, e.Kind, want)
		}
	}
	if e, _ := s.CacheGet("person/1"); e == nil || e.Kind != "people" {
		t.Errorf("person page changed kind")
	}
}

// Flushing some kinds leaves the others; flushing with no kind clears everything.
func TestCacheFlushKinds(t *testing.T) {
	dir := t.TempDir()
	box, _ := secret.Load(dir)
	s, err := Open(dir, box)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	exp := time.Now().Add(time.Hour)
	for k, kind := range map[string]string{"movie/1": "movies", "series/2": "series", "series/2/season/1": "seasons", "collection/3": "collections", "person/4": "people", "lists/trending": "lists"} {
		s.CachePut(k, CacheEntry{Kind: kind, Status: 200, Body: []byte(`{}`), Expires: exp})
	}
	if n, err := s.CacheFlush("movies", "series", "seasons", "collections"); err != nil || n != 4 {
		t.Fatalf("flushed %d (%v), want 4", n, err)
	}
	stats, _ := s.CacheStats()
	if len(stats) != 2 {
		t.Fatalf("left %+v, want people and lists", stats)
	}
	if n, _ := s.CacheFlush("seasons"); n != 0 {
		t.Fatalf("flushed %d seasons, want 0", n)
	}
	if n, _ := s.CacheFlush(); n != 2 {
		t.Fatalf("flush all removed %d, want 2", n)
	}
}
