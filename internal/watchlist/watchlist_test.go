package watchlist

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/Flixarr/flixarr/internal/requests"
	"github.com/Flixarr/flixarr/internal/secret"
	"github.com/Flixarr/flixarr/internal/store"
)

type noMeta struct{}

func (noMeta) Get(context.Context, string, url.Values) ([]byte, int, error) { return nil, 404, nil }

func TestWatchlistSync(t *testing.T) {
	dir := t.TempDir()
	box, _ := secret.Load(dir)
	st, _ := store.Open(dir, box)
	defer st.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Plex-Token") != "eryn-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		fmt.Fprint(w, `{"MediaContainer":{"totalSize":3,"Metadata":[
			{"type":"movie","title":"In Plex","Guid":[{"id":"imdb://tt1"},{"id":"tmdb://603"}]},
			{"type":"show","title":"Some Show","Guid":[{"id":"tmdb://1396"}]},
			{"type":"movie","title":"No TMDB","Guid":[{"id":"imdb://tt2"}]}]}}`)
	}))
	defer srv.Close()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	w := &Syncer{Store: st, Requests: &requests.Service{Store: st, Log: log}, Meta: noMeta{}, Log: log, Base: srv.URL}
	st.CreateUser(store.User{PlexID: 1, Username: "owner", Role: "Admin"})
	eryn, _ := st.CreateUser(store.User{PlexID: 2, Username: "eryn", Perms: append(store.RolePerms["User"], "watchlist")}) // Watchlist is never in a preset
	st.SetPlexToken(eryn.ID, "eryn-token")
	st.SavePlexItem(store.PlexItem{Type: "movie", TmdbID: 603, RatingKey: "1", Section: "1", SeenAt: "x"})

	items, err := w.Fetch(context.Background(), "eryn-token")
	if err != nil || len(items) != 2 || items[1].Type != "series" {
		t.Fatalf("fetch: %+v %v", items, err)
	}
	// Off: nothing happens.
	if msg, _ := w.Run(context.Background()); msg != "0 titles requested from 0 watchlists" {
		t.Fatalf("off: %q", msg)
	}
	st.SetWatchlist(eryn.ID, true)
	w.Run(context.Background()) // no Sonarr connected: the show fails, the movie is in Plex
	if !st.WatchlistSeen(eryn.ID, "movie", 603) || !st.WatchlistSeen(eryn.ID, "series", 1396) {
		t.Fatal("titles not marked as handled")
	}
	// Without the permission, nothing is fetched.
	st.SetPerms(eryn.ID, []string{"requestMovies"})
	if msg, _ := w.Run(context.Background()); msg != "0 titles requested from 0 watchlists" {
		t.Fatalf("no permission: %q", msg)
	}
}
