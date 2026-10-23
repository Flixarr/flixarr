package images

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Flixarr/flixarr/internal/secret"
	"github.com/Flixarr/flixarr/internal/store"
)

func newCache(t *testing.T) *Cache {
	t.Helper()
	dir := t.TempDir()
	box, _ := secret.Load(dir)
	st, err := store.Open(dir, box)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return New(dir, st)
}

func TestValid(t *testing.T) {
	for _, c := range []struct {
		size, file string
		ok         bool
	}{
		{"w500", "abc123.jpg", true},
		{"original", "x_y-z.png", true},
		{"w999", "abc.jpg", false},
		{"w500", "../etc/passwd", false},
		{"w500", "abc.exe", false},
		{"w500", "", false},
	} {
		if Valid(c.size, c.file) != c.ok {
			t.Errorf("Valid(%q, %q) = %v", c.size, c.file, !c.ok)
		}
	}
}

func TestAvatarValid(t *testing.T) {
	for u, ok := range map[string]bool{
		"https://plex.tv/users/abc/avatar?c=1":    true,
		"https://images.plex.tv/photo?size=small": true,
		"http://plex.tv/users/abc/avatar":         false, // https only
		"https://evilplex.tv/a":                   false,
		"https://plex.tv.evil.com/a":              false,
		"https://user@plex.tv/a":                  false,
		"not a url":                               false,
		"":                                        false,
	} {
		if AvatarValid(u) != ok {
			t.Errorf("AvatarValid(%q) = %v", u, !ok)
		}
	}
}

// get downloads once, then serves the saved copy, counting a miss then a hit.
func TestGetSavesAndCounts(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write([]byte("jpegdata"))
	}))
	defer srv.Close()
	c := newCache(t)
	p := filepath.Join(c.Dir, "w500", "a.jpg")
	for i := 0; i < 2; i++ {
		got, err := c.get(context.Background(), &c.Images, p, srv.URL)
		if err != nil || got != p {
			t.Fatalf("get: %q %v", got, err)
		}
	}
	if b, _ := os.ReadFile(p); string(b) != "jpegdata" {
		t.Fatalf("saved %q", b)
	}
	if calls != 1 {
		t.Fatalf("downloaded %d times", calls)
	}
	if hm := c.Images.Load(); hm.Hits != 1 || hm.Misses != 1 {
		t.Fatalf("counters %+v", hm)
	}
	if s := c.Stats(); s.Count != 1 || s.Bytes != 8 {
		t.Fatalf("stats %+v", s)
	}
}

func TestGetRefusesBadAnswers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/html" {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte("<html>"))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	c := newCache(t)
	for _, path := range []string{"/html", "/missing"} {
		p := filepath.Join(c.AvatarDir, "x"+path[1:])
		if _, err := c.get(context.Background(), &c.Avatars, p, srv.URL+path); err == nil {
			t.Errorf("%s: no error", path)
		}
		if _, err := os.Stat(p); err == nil {
			t.Errorf("%s: file saved", path)
		}
	}
}

func TestAvatarPathRejectsOtherHosts(t *testing.T) {
	c := newCache(t)
	if _, err := c.AvatarPath(context.Background(), "https://example.com/a.png"); err == nil {
		t.Fatal("non-Plex avatar accepted")
	}
}

// Cleanup removes images and avatars nobody viewed within Days(), and keeps recent ones.
func TestCleanupAndFlush(t *testing.T) {
	c := newCache(t)
	old := time.Now().AddDate(0, 0, -(DefaultDays + 1))
	write := func(p string, mod time.Time) {
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte("x"), 0o644)
		os.Chtimes(p, mod, mod)
	}
	write(filepath.Join(c.Dir, "w500", "old.jpg"), old)
	write(filepath.Join(c.Dir, "w500", "new.jpg"), time.Now())
	write(filepath.Join(c.AvatarDir, "oldavatar"), old)
	write(filepath.Join(c.AvatarDir, "newavatar"), time.Now())

	n, err := c.Cleanup()
	if err != nil || n != 2 {
		t.Fatalf("cleanup removed %d, %v", n, err)
	}
	if c.Stats().Count != 1 || c.AvatarStats().Count != 1 {
		t.Fatalf("left %d images, %d avatars", c.Stats().Count, c.AvatarStats().Count)
	}
	if n, _ := c.FlushAvatars(); n != 1 || c.AvatarStats().Count != 0 || c.Stats().Count != 1 {
		t.Fatal("flushing avatars touched images or left avatars")
	}
	if n, _ := c.Flush(); n != 1 || c.Stats().Count != 0 {
		t.Fatal("flush left images")
	}
}

func TestCleanupWithNothingSaved(t *testing.T) {
	c := newCache(t)
	if n, err := c.Cleanup(); n != 0 || err != nil {
		t.Fatalf("%d %v", n, err)
	}
}

func TestSetConfig(t *testing.T) {
	c := newCache(t)
	if c.Enabled() || c.Days() != DefaultDays {
		t.Fatal("defaults")
	}
	if err := c.SetConfig(true, 0); err == nil {
		t.Fatal("0 days accepted")
	}
	if err := c.SetConfig(true, 14); err != nil || !c.Enabled() || c.Days() != 14 {
		t.Fatalf("set: %v", err)
	}
}
