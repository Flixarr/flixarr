// Package watchlist turns titles people add to their Plex Watchlist into Flixarr requests, for people
// who switched it on in their Profile (and have the Plex Watchlist requests permission).
package watchlist

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Flixarr/flixarr/internal/requests"
	"github.com/Flixarr/flixarr/internal/store"
)

// Meta fetches Flixarr API detail pages (meta.Client).
type Meta interface {
	Get(ctx context.Context, path string, q url.Values) ([]byte, int, error)
}

// Syncer is the "Plex Watchlist: sync" job.
type Syncer struct {
	Store    *store.Store
	Requests *requests.Service
	Meta     Meta
	ClientID string
	Log      *slog.Logger
	HTTP     *http.Client
	Base     string // discover.provider.plex.tv, for tests
}

// Item is a watchlist title.
type Item struct {
	Type   string // movie | series
	TmdbID int
	Title  string
}

// TokenFor is the user's plex.tv token; the owner's falls back to the stored admin token (from before
// per-user tokens were kept).
func (w *Syncer) TokenFor(u *store.User) string {
	if t := w.Store.PlexToken(u.ID); t != "" {
		return t
	}
	if u.Owner {
		return w.Store.SecretSetting("plex_admin_token", "")
	}
	return ""
}

// Fetch reads a watchlist from plex.tv.
func (w *Syncer) Fetch(ctx context.Context, token string) ([]Item, error) {
	base := w.Base
	if base == "" {
		base = "https://discover.provider.plex.tv"
	}
	hc := w.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 20 * time.Second}
	}
	out := []Item{}
	for start := 0; start < 2000; start += 100 {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/library/sections/watchlist/all?includeGuids=1&X-Plex-Container-Start=%d&X-Plex-Container-Size=100", base, start), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("X-Plex-Token", token)
		req.Header.Set("X-Plex-Client-Identifier", w.ClientID)
		req.Header.Set("X-Plex-Product", "Flixarr")
		res, err := hc.Do(req)
		if err != nil {
			return nil, errors.New("couldn't reach plex.tv")
		}
		var page struct {
			MediaContainer struct {
				TotalSize int `json:"totalSize"`
				Metadata  []struct {
					Type  string `json:"type"`
					Title string `json:"title"`
					Guid  []struct {
						ID string `json:"id"`
					} `json:"Guid"`
				} `json:"Metadata"`
			} `json:"MediaContainer"`
		}
		err = json.NewDecoder(res.Body).Decode(&page)
		res.Body.Close()
		if res.StatusCode == http.StatusUnauthorized {
			return nil, errors.New("plex.tv refused the sign-in; sign in to Flixarr again")
		}
		if res.StatusCode >= 300 || err != nil {
			return nil, fmt.Errorf("plex.tv answered %s", res.Status)
		}
		for _, m := range page.MediaContainer.Metadata {
			typ := map[string]string{"movie": "movie", "show": "series"}[m.Type]
			for _, g := range m.Guid {
				if id, ok := strings.CutPrefix(g.ID, "tmdb://"); ok && typ != "" {
					if n, _ := strconv.Atoi(id); n > 0 {
						out = append(out, Item{Type: typ, TmdbID: n, Title: m.Title})
					}
				}
			}
		}
		if len(page.MediaContainer.Metadata) == 0 || start+100 >= page.MediaContainer.TotalSize {
			break
		}
	}
	return out, nil
}

type detail struct {
	Title    string   `json:"title"`
	Year     string   `json:"year"`
	Poster   string   `json:"poster"`
	Backdrop string   `json:"backdrop"`
	Genres   []string `json:"genres"`
	Language string   `json:"language"`
	Seasons  []struct {
		N int `json:"n"`
	} `json:"seasons"`
}

// Run is the job: for everyone with it on, request watchlist titles not handled yet.
func (w *Syncer) Run(ctx context.Context) (string, error) {
	users, err := w.Store.WatchlistUsers()
	if err != nil {
		return "", err
	}
	made, people := 0, 0
	var firstErr error
	for _, u := range users {
		if !u.Can("watchlist") {
			continue
		}
		tok := w.TokenFor(u)
		if tok == "" {
			continue // signs in again → token kept
		}
		items, err := w.Fetch(ctx, tok)
		if err != nil {
			w.Log.Warn("watchlist", "user", u.Username, "err", err)
			if firstErr == nil {
				firstErr = fmt.Errorf("%s: %w", u.Username, err)
			}
			continue
		}
		people++
		for _, it := range items {
			if w.Store.WatchlistSeen(u.ID, it.Type, it.TmdbID) {
				continue
			}
			made += w.handle(ctx, u, it)
		}
	}
	msg := fmt.Sprintf("%d %s requested from %d %s", made, plural(made, "title", "titles"), people, plural(people, "watchlist", "watchlists"))
	return msg, firstErr
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// handle requests one title for a user and records the outcome so it's never tried twice.
func (w *Syncer) handle(ctx context.Context, u *store.User, it Item) int {
	key := fmt.Sprintf("%s:%d", it.Type, it.TmdbID)
	if got, _ := w.Store.PlexItemsFor([]string{key}); len(got) > 0 {
		w.Store.MarkWatchlistSeen(u.ID, it.Type, it.TmdbID, "in-library")
		return 0
	}
	if _, open, _ := w.Store.OpenRequest(it.Type, it.TmdbID); open {
		w.Store.MarkWatchlistSeen(u.ID, it.Type, it.TmdbID, "already-requested")
		return 0
	}
	in := requests.New{Type: it.Type, TmdbID: it.TmdbID, Title: it.Title}
	q := url.Values{}
	q.Set("region", w.Store.Setting("region", "US"))
	q.Set("language", w.Store.Setting("language", "en-US"))
	if body, status, err := w.Meta.Get(ctx, fmt.Sprintf("%s/%d", it.Type, it.TmdbID), q); err == nil && status == http.StatusOK {
		var d detail
		if json.Unmarshal(body, &d) == nil {
			in.Title, in.Year, in.Poster, in.Backdrop, in.Genres, in.Language = d.Title, d.Year, d.Poster, d.Backdrop, d.Genres, d.Language
			for _, s := range d.Seasons {
				if s.N > 0 {
					in.SeasonCount++
				}
			}
		}
	}
	r, err := w.Requests.Create(ctx, u, in)
	var limit requests.LimitError
	switch {
	case errors.As(err, &limit):
		return 0 // over this week's limit: try again on a later run
	case err != nil:
		w.Store.MarkWatchlistSeen(u.ID, it.Type, it.TmdbID, "failed: "+err.Error())
		return 0
	}
	w.Store.MarkWatchlistSeen(u.ID, it.Type, it.TmdbID, "requested")
	w.Log.Info("watchlist request", "user", u.Username, "title", r.Title, "status", r.Status)
	return 1
}
