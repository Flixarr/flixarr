// Package images saves TMDB posters and backdrops on disk (Settings → Cache → Image caching),
// so browsers load them from this server instead of image.tmdb.org. People's Plex avatars are
// always saved too: plex.tv answers every avatar with an uncached redirect.
package images

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Flixarr/flixarr/internal/meta"
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

// Cache stores images under <data>/cache/images/<size>/<file> and avatars under <data>/cache/avatars.
type Cache struct {
	Dir       string
	AvatarDir string
	Store     *store.Store
	HTTP      *http.Client

	Images  meta.HitMissCounter // posters and backdrops: served from disk / downloaded
	Avatars meta.HitMissCounter
}

// New returns an image cache rooted in dataDir.
func New(dataDir string, st *store.Store) *Cache {
	return &Cache{Dir: filepath.Join(dataDir, "cache", "images"), AvatarDir: filepath.Join(dataDir, "cache", "avatars"),
		Store: st, HTTP: &http.Client{Timeout: 30 * time.Second}}
}

// Enabled reports whether image caching is on (off by default, like Seerr).
func (c *Cache) Enabled() bool { return c.Store.Setting(keyEnabled, "false") == "true" }

// DefaultDays is how long an unviewed image is kept on a new install.
const DefaultDays = 90

// Days is how long an image nobody views is kept.
func (c *Cache) Days() int {
	if n, err := strconv.Atoi(c.Store.Setting(keyDays, "")); err == nil && n > 0 {
		return n
	}
	return DefaultDays
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
	return c.get(ctx, &c.Images, filepath.Join(c.Dir, size, file), tmdbBase+size+"/"+file)
}

// AvatarValid reports whether u is a Plex avatar address (https on plex.tv), the only avatars saved.
func AvatarValid(u string) bool {
	p, err := url.Parse(u)
	if err != nil || p.Scheme != "https" || p.User != nil {
		return false
	}
	h := p.Hostname()
	return h == "plex.tv" || strings.HasSuffix(h, ".plex.tv")
}

// AvatarPath returns the saved copy of a Plex avatar, downloading it first if needed. The address
// changes when someone changes their picture (plex.tv adds ?c=<time>), so a new picture is a new file.
func (c *Cache) AvatarPath(ctx context.Context, u string) (string, error) {
	if !AvatarValid(u) {
		return "", errors.New("not a Plex avatar")
	}
	sum := sha256.Sum256([]byte(u))
	return c.get(ctx, &c.Avatars, filepath.Join(c.AvatarDir, hex.EncodeToString(sum[:16])), u)
}

// get returns p, downloading src into it first if it isn't saved yet.
func (c *Cache) get(ctx context.Context, n *meta.HitMissCounter, p, src string) (string, error) {
	if _, err := os.Stat(p); err == nil {
		now := time.Now()
		os.Chtimes(p, now, now) // "last viewed", so cleanup keeps images people still look at
		n.Hit()
		return p, nil
	}
	n.Miss()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src, nil)
	if err != nil {
		return "", err
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("image download: %s", res.Status)
	}
	if ct := res.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(ct, "image/") {
		return "", fmt.Errorf("image download: not an image (%s)", ct)
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

// Stats counts the saved posters and backdrops.
func (c *Cache) Stats() Stats { return dirStats(c.Dir) }

// AvatarStats counts the saved avatars.
func (c *Cache) AvatarStats() Stats { return dirStats(c.AvatarDir) }

func dirStats(dir string) Stats {
	var s Stats
	filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
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

// Flush deletes every saved poster and backdrop.
func (c *Cache) Flush() (int, error) {
	n := c.Stats().Count
	return n, os.RemoveAll(c.Dir)
}

// FlushAvatars deletes every saved avatar (they download again on the next view).
func (c *Cache) FlushAvatars() (int, error) {
	n := c.AvatarStats().Count
	return n, os.RemoveAll(c.AvatarDir)
}

// Cleanup deletes images and avatars nobody has viewed in Days() days.
func (c *Cache) Cleanup() (int, error) {
	cutoff := time.Now().AddDate(0, 0, -c.Days())
	n := 0
	for _, dir := range []string{c.Dir, c.AvatarDir} {
		err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
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
		if err != nil {
			return n, err
		}
	}
	return n, nil
}
