package plexlib

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/Flixarr/flixarr/internal/secret"
	"github.com/Flixarr/flixarr/internal/store"
)

type fakeMeta map[string]string

func (f fakeMeta) Get(_ context.Context, path string, _ url.Values) ([]byte, int, error) {
	if b, ok := f[path]; ok {
		return []byte(b), http.StatusOK, nil
	}
	return nil, http.StatusNotFound, nil
}

// Whole or part of a series (#137): specials and seasons that haven't started don't count, and a season
// still airing counts only the episodes already out.
func TestSeriesEpisodes(t *testing.T) {
	dir := t.TempDir()
	box, _ := secret.Load(dir)
	st, _ := store.Open(dir, box)
	defer st.Close()
	plex := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/library/metadata/7/children" {
			// Plex has: 2 specials, all 10 of season 1, 3 of season 2.
			fmt.Fprint(w, `{"MediaContainer":{"Metadata":[{"index":0,"leafCount":2},{"index":1,"leafCount":10},{"index":2,"leafCount":3}]}}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer plex.Close()
	st.SetSetting(keyServerURL, plex.URL)
	st.SetSecretSetting(keyToken, "srv")
	st.SavePlexItem(store.PlexItem{Type: "series", TmdbID: 1, RatingKey: "7", Section: "2", SeenAt: "x"})
	l := &Library{Store: st, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Meta: fakeMeta{
		// Season 2 is airing (8 episodes planned, 4 out by 2026-10-20), season 3 starts next year.
		"series/1":          `{"seasons":[{"n":1,"episodes":10,"air":"2024-01-01"},{"n":2,"episodes":8,"air":"2026-10-01"},{"n":3,"episodes":6,"air":"2027-03-01"}],"next":"2026-10-27"}`,
		"series/1/season/2": `{"episodes":[{"air":"2026-10-01"},{"air":"2026-10-08"},{"air":"2026-10-15"},{"air":"2026-10-20"},{"air":"2026-10-27"},{"air":null},{"air":null},{"air":null}]}`,
	}}
	have, total, err := l.seriesEpisodes(context.Background(), 1, "2026-10-20")
	if err != nil || have != 13 || total != 14 {
		t.Fatalf("got %d of %d (%v), want 13 of 14", have, total, err)
	}
	// Once season 2 has finished airing, all 8 count.
	l.Meta = fakeMeta{"series/1": `{"seasons":[{"n":1,"episodes":10,"air":"2024-01-01"},{"n":2,"episodes":8,"air":"2026-10-01"}],"next":null}`}
	if have, total, _ = l.seriesEpisodes(context.Background(), 1, "2027-01-01"); have != 13 || total != 18 {
		t.Fatalf("ended: %d of %d", have, total)
	}
}
