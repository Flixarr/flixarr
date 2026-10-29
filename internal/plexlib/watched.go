package plexlib

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path"
	"strings"
)

// WatchedItem is a movie or series someone watched lately in Plex (a series once, for its newest episode).
type WatchedItem struct {
	Type   string // movie | series
	TmdbID int
	Title  string
	At     int64 // when, unix seconds
}

// Watched is what one person watched lately on the connected server, newest first, as titles Flixarr knows
// from Plex (#181). plexID is their plex.tv account; the server's owner shows up in Plex's history as
// account 1, so owner says to use that when the server has no account under their plex.tv ID.
func (l *Library) Watched(ctx context.Context, plexID int64, owner bool, limit int) ([]WatchedItem, error) {
	base, tok, ok := l.conn()
	if !ok {
		return nil, nil
	}
	var accounts struct {
		MediaContainer struct {
			Account []struct {
				ID int64 `json:"id"`
			} `json:"Account"`
		} `json:"MediaContainer"`
	}
	if err := l.get(ctx, base, "/accounts", tok, &accounts); err != nil {
		return nil, err
	}
	account := int64(-1)
	for _, a := range accounts.MediaContainer.Account {
		if plexID > 0 && a.ID == plexID {
			account = a.ID
		}
	}
	if account < 0 && owner {
		account = 1
	}
	if account < 0 {
		return nil, nil // not someone this server knows
	}
	var history struct {
		MediaContainer struct {
			Metadata []struct {
				Type           string `json:"type"`
				RatingKey      string `json:"ratingKey"`
				GrandparentKey string `json:"grandparentKey"`
				ViewedAt       int64  `json:"viewedAt"`
			} `json:"Metadata"`
		} `json:"MediaContainer"`
	}
	q := fmt.Sprintf("/status/sessions/history/all?accountID=%d&sort=viewedAt:desc", account)
	if err := l.getPage(ctx, base, q, tok, 200, &history); err != nil {
		return nil, err
	}
	out, seen := []WatchedItem{}, map[string]bool{}
	for _, h := range history.MediaContainer.Metadata {
		key := h.RatingKey
		if h.Type == "episode" {
			key = path.Base(h.GrandparentKey) // the show: /library/metadata/52027
		} else if h.Type != "movie" {
			continue
		}
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		it, ok := l.Store.PlexItemByRatingKey(key)
		if !ok {
			continue // gone from Plex, or in a library Flixarr doesn't follow
		}
		out = append(out, WatchedItem{Type: it.Type, TmdbID: it.TmdbID, Title: it.Title, At: h.ViewedAt})
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

// getPage is get for Plex lists too long to read whole: only their first size entries.
func (l *Library) getPage(ctx context.Context, base, p, token string, size int, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+p, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Plex-Token", token)
	req.Header.Set("X-Plex-Product", "Flixarr")
	req.Header.Set("X-Plex-Version", l.Version)
	req.Header.Set("X-Plex-Client-Identifier", l.ClientID)
	req.Header.Set("X-Plex-Container-Start", "0")
	req.Header.Set("X-Plex-Container-Size", fmt.Sprint(size))
	res, err := l.client().Do(req)
	if err != nil {
		return errors.New("couldn't reach it")
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		return fmt.Errorf("Plex answered %s", res.Status)
	}
	return json.NewDecoder(res.Body).Decode(out)
}
