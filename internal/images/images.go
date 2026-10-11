// Package images saves TMDB posters and backdrops on disk (Settings → Cache → Image caching),
// so browsers load them from this server instead of image.tmdb.org.
package images

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"github.com/Flixarr/flixarr/internal/store"
)

const (
	tmdbBase   = "https://image.tmdb.org/t/p/"
	keyEnabled = "image_cache_enabled"
	keyDays    = "image_cache_days"
)

// Sizes TMDB serves (anything else is rejected).
var sizes = map[string]bool{"w45": true, "w92": true, "w154": true, "w185": true, "w300": true, "w342": true,
	"w500": true, "w780": true, "w1280": true, "h632": true, "original": true}

var fileRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}\.(jpg|jpeg|png|webp|svg)$`)

// Cache stores images under <data>/cache/images/<size>/<file>.
type Cache struct {
	Dir   string
	Store *store.Store
	HTTP  *http.Client
}

// New returns an image cache rooted in dataDir.
func New(dataDir string, st *store.Store) *Cache {
	return &Cache{Dir: filepath.Join(dataDir, "cache", "images"), Store: st, HTTP: &http.Client{Timeout: 30 * time.Second}}
}

// Enabled reports whether image caching is on (off by default, like Seerr).
func (c *Cache) Enabled() bool { return c.Store.Setting(keyEnabled, "false") == "true" }

// Days is how long an image nobody views is kept.
func (c *Cache) Days() int {
	if n, err := strconv.Atoi(c.Store.Setting(keyDays, "")); err == nil && n > 0 {
		return n
	}
	return 30
}

// SetConfig saves the image cache settings.
func (c *Cache) SetConfig(enabled bool, days int) error {
	if days < 1 || days > 365 {
		return errors.New("keep unused images between 1 and 365 days")
	}
	if err := c.Store.SetSetting(keyEnabled, strconv.FormatBool(enabled)); err != nil {
		return err
	}
	return c.Store.SetSetting(keyDays, strconv.Itoa(days))
}

// Valid reports whether size/file is a real TMDB image path.
func Valid(size, file string) bool { return sizes[size] && fileRe.MatchString(file) }

// Path returns the image's file, downloading it from TMDB first if it isn't saved yet.
func (c *Cache) Path(ctx context.Context, size, file string) (string, error) {
	p := filepath.Join(c.Dir, size, file)
	if _, err := os.Stat(p); err == nil {
		now := time.Now()
		os.Chtimes(p, now, now) // "last viewed", so cleanup keeps images people still look at
		return p, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tmdbBase+size+"/"+file, nil)
	if err != nil {
		return "", err
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("TMDB image: %s", res.Status)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".dl-*")
	if err != nil {
		return "", err
	}
	_, err = io.Copy(tmp, io.LimitReader(res.Body, 20<<20))
	tmp.Close()
	if err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	if err := os.Rename(tmp.Name(), p); err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	return p, nil
}

// Stats is how many images are saved and how much space they use.
type Stats struct {
	Count int   `json:"count"`
	Bytes int64 `json:"bytes"`
}

// Stats counts the saved images.
func (c *Cache) Stats() Stats {
	var s Stats
	filepath.WalkDir(c.Dir, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if info, err := d.Info(); err == nil {
				s.Count++
				s.Bytes += info.Size()
			}
		}
		return nil
	})
	return s
}

// Flush deletes every saved image.
func (c *Cache) Flush() (int, error) {
	n := c.Stats().Count
	return n, os.RemoveAll(c.Dir)
}

// Cleanup deletes images nobody has viewed in Days() days.
func (c *Cache) Cleanup() (int, error) {
	cutoff := time.Now().AddDate(0, 0, -c.Days())
	n := 0
	err := filepath.WalkDir(c.Dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		if info, err := d.Info(); err == nil && info.ModTime().Before(cutoff) {
			if os.Remove(p) == nil {
				n++
			}
		}
		return nil
	})
	return n, err
}
