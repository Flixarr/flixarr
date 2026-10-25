package plexlib

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/Flixarr/flixarr/internal/store"
)

// FileItem is the Plex movie that holds a file, as the last full scan saw it (#171).
type FileItem struct {
	RatingKey string
	Title     string
	Year      int
	TmdbID    int // 0 when Plex matched it to something with no TMDB ID
}

// fileKey names a file by its folder and file name only: Plex and Radarr usually see the same files under
// different mount points.
func fileKey(p string) string {
	p = strings.ToLower(strings.ReplaceAll(p, `\`, "/"))
	return path.Base(path.Dir(p)) + "/" + path.Base(p)
}

// rememberFiles keeps which Plex movie holds each file, replacing the list from the last full scan.
func (l *Library) rememberFiles(files map[string]FileItem) {
	l.filesMu.Lock()
	l.files = files
	l.filesMu.Unlock()
}

// FileItem finds the Plex movie that holds a file (a path as Radarr gives it).
func (l *Library) FileItem(file string) (FileItem, bool) {
	l.filesMu.Lock()
	defer l.filesMu.Unlock()
	it, ok := l.files[fileKey(file)]
	return it, ok
}

// FixMatch tells Plex which movie an item really is (by its TMDB ID), then reads the item again until Plex
// has caught up, so Flixarr sees it in Plex without waiting for the next scan.
func (l *Library) FixMatch(ctx context.Context, ratingKey string, tmdbID int) error {
	base, tok, ok := l.conn()
	if !ok {
		return errors.New("no Plex server is connected")
	}
	var res struct {
		MediaContainer struct {
			SearchResult []struct {
				GUID string `json:"guid"`
				Name string `json:"name"`
				Year int    `json:"year"`
			} `json:"SearchResult"`
		} `json:"MediaContainer"`
	}
	item := "/library/metadata/" + url.PathEscape(ratingKey)
	if err := l.get(ctx, base, fmt.Sprintf("%s/matches?manual=1&title=tmdb-%d", item, tmdbID), tok, &res); err != nil {
		return err
	}
	if len(res.MediaContainer.SearchResult) == 0 {
		return errors.New("Plex couldn't find that movie")
	}
	m := res.MediaContainer.SearchResult[0]
	q := url.Values{"guid": {m.GUID}, "name": {m.Name}}
	if m.Year > 0 {
		q.Set("year", fmt.Sprint(m.Year))
	}
	var out any
	if err := l.do(ctx, http.MethodPut, base, item+"/match?"+q.Encode(), tok, &out); err != nil {
		return err
	}
	go l.reread(ratingKey, tmdbID)
	return nil
}

// reread reads one item again while Plex refreshes it after a new match, and saves it once it has the
// TMDB ID it was matched to.
func (l *Library) reread(ratingKey string, tmdbID int) {
	base, tok, ok := l.conn()
	if !ok {
		return
	}
	for _, wait := range []time.Duration{3 * time.Second, 10 * time.Second, 30 * time.Second, time.Minute} {
		time.Sleep(wait)
		var res struct {
			MediaContainer struct {
				Metadata []metadata `json:"Metadata"`
			} `json:"MediaContainer"`
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		err := l.get(ctx, base, "/library/metadata/"+url.PathEscape(ratingKey)+"?includeGuids=1", tok, &res)
		cancel()
		if err != nil || len(res.MediaContainer.Metadata) == 0 {
			continue
		}
		m := res.MediaContainer.Metadata[0]
		if m.tmdb() != tmdbID {
			continue
		}
		l.Store.SavePlexItem(store.PlexItem{Type: "movie", TmdbID: tmdbID, RatingKey: m.RatingKey, Section: fmt.Sprint(m.LibrarySectionID), Title: m.Title, AddedAt: m.AddedAt, SeenAt: stamp()})
		l.filesMu.Lock()
		for k, it := range l.files {
			if it.RatingKey == ratingKey {
				l.files[k] = FileItem{RatingKey: ratingKey, Title: m.Title, Year: m.Year, TmdbID: tmdbID}
			}
		}
		l.filesMu.Unlock()
		if l.OnAdded != nil {
			l.OnAdded([]string{fmt.Sprintf("movie:%d", tmdbID)})
		}
		return
	}
}
