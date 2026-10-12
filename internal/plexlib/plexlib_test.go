package plexlib

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Flixarr/flixarr/internal/secret"
	"github.com/Flixarr/flixarr/internal/store"
)

// fakePlex is plex.tv and a media server in one.
type fakePlex struct {
	url    string
	movies []string // tmdb IDs in section 1
}

func (f *fakePlex) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	tok := r.Header.Get("X-Plex-Token")
	switch {
	case r.URL.Path == "/api/v2/resources":
		if tok != "admin" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		json.NewEncoder(w).Encode([]any{
			map[string]any{"name": "Phone", "provides": "player", "clientIdentifier": "p"},
			map[string]any{"name": "Friend's", "provides": "server", "owned": false, "clientIdentifier": "f", "accessToken": "x"},
			map[string]any{"name": "Home", "provides": "server", "owned": true, "clientIdentifier": "abc", "accessToken": "srv",
				"connections": []any{map[string]any{"uri": "http://127.0.0.1:1", "local": false, "relay": true}, map[string]any{"uri": f.url, "local": true}}},
		})
	case tok != "srv":
		w.WriteHeader(http.StatusUnauthorized)
	case r.URL.Path == "/identity":
		fmt.Fprint(w, `{"MediaContainer":{"machineIdentifier":"abc"}}`)
	case r.URL.Path == "/library/sections":
		fmt.Fprint(w, `{"MediaContainer":{"Directory":[{"key":"1","type":"movie","title":"Movies","agent":"tv.plex.agents.movie"},{"key":"2","type":"show","title":"TV Shows","agent":"tv.plex.agents.series"},{"key":"4","type":"movie","title":"Home Videos","agent":"com.plexapp.agents.none"},{"key":"5","type":"artist","title":"Music"}]}}`)
	case r.URL.Path == "/library/sections/1/all":
		var md []any
		for i, id := range f.movies {
			md = append(md, map[string]any{"ratingKey": fmt.Sprint(100 + i), "title": "M" + id, "guid": "plex://movie/x", "Guid": []any{map[string]any{"id": "imdb://tt1"}, map[string]any{"id": "tmdb://" + id}}})
		}
		md = append(md, map[string]any{"ratingKey": "999", "title": "No TMDB"})
		json.NewEncoder(w).Encode(map[string]any{"MediaContainer": map[string]any{"totalSize": len(md), "Metadata": md}})
	case r.URL.Path == "/library/sections/2/all":
		fmt.Fprint(w, `{"MediaContainer":{"totalSize":1,"Metadata":[{"ratingKey":"7","title":"Breaking Bad","guid":"plex://show/y","Guid":[{"id":"tmdb://1396"}]}]}}`)
	default:
		if strings.HasPrefix(r.URL.Path, "/library/sections/4") {
			panic("home videos should not be scanned")
		}
		w.WriteHeader(http.StatusNotFound)
	}
}

func TestConnectScanLookup(t *testing.T) {
	dir := t.TempDir()
	box, _ := secret.Load(dir)
	st, err := store.Open(dir, box)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	f := &fakePlex{movies: []string{"603", "604"}}
	srv := httptest.NewServer(f)
	defer srv.Close()
	f.url = srv.URL
	l := &Library{Store: st, ClientID: "c", Version: "t", Log: slog.New(slog.NewTextHandler(io.Discard, nil)), TVBaseURL: srv.URL}
	ctx := context.Background()

	if _, err := l.Servers(ctx); err == nil {
		t.Fatal("servers without a token should fail")
	}
	st.SetSecretSetting(keyAdminToken, "admin")
	list, err := l.Servers(ctx)
	if err != nil || len(list) != 1 || list[0].Connections[0].URI != srv.URL {
		t.Fatalf("servers: %+v %v", list, err)
	}
	stt, err := l.Connect(ctx, "abc")
	if err != nil || !stt.Connected || stt.URL != srv.URL || len(stt.Sections) != 3 {
		t.Fatalf("connect: %+v %v", stt, err)
	}
	if st.Setting(keyToken, "") == "srv" {
		t.Fatal("server token stored in plain text")
	}
	for _, s := range stt.Sections {
		if s.Enabled != (s.Key != "4") {
			t.Fatalf("default sections: %+v", stt.Sections)
		}
	}
	msg, err := l.FullScan(ctx)
	if err != nil || !strings.HasPrefix(msg, "3 titles") {
		t.Fatalf("scan: %q %v", msg, err)
	}
	got, _ := l.Lookup([]string{"movie:603", "series:1396", "movie:1"})
	if len(got) != 2 || !strings.Contains(got["movie:603"].URL, "server/abc/details?key=%2Flibrary%2Fmetadata%2F100") {
		t.Fatalf("lookup: %+v", got)
	}
	// Removed from Plex: gone after the next full scan.
	f.movies = []string{"603"}
	if msg, _ = l.FullScan(ctx); !strings.Contains(msg, "1 removed") {
		t.Fatalf("rescan: %q", msg)
	}
	// Only TV chosen: movies drop out.
	l.SetSections([]string{"2"})
	l.FullScan(ctx)
	if got, _ = l.Lookup([]string{"movie:603", "series:1396"}); len(got) != 1 {
		t.Fatalf("after choosing TV only: %+v", got)
	}
	l.Disconnect()
	if stt, _ = l.Status(ctx); stt.Connected {
		t.Fatal("still connected")
	}
}

func TestFriendsAndAccess(t *testing.T) {
	dir := t.TempDir()
	box, _ := secret.Load(dir)
	st, _ := store.Open(dir, box)
	defer st.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/users":
			fmt.Fprint(w, `<?xml version="1.0"?><MediaContainer>
<User id="11" title="Eryn" username="eryn" email="e@x" thumb="t"><Server machineIdentifier="abc" pending="0"/></User>
<User id="12" title="Other" username="other"><Server machineIdentifier="zzz" pending="0"/></User>
<User id="13" title="Invited" username="inv"><Server machineIdentifier="abc" pending="1"/></User>
</MediaContainer>`)
		case "/api/v2/resources":
			if r.Header.Get("X-Plex-Token") == "friend" {
				fmt.Fprint(w, `[{"clientIdentifier":"abc"}]`)
			} else {
				fmt.Fprint(w, `[{"clientIdentifier":"zzz"}]`)
			}
		}
	}))
	defer srv.Close()
	l := &Library{Store: st, ClientID: "c", Log: slog.New(slog.NewTextHandler(io.Discard, nil)), TVBaseURL: srv.URL}
	ctx := context.Background()
	if _, err := l.Friends(ctx); err == nil {
		t.Fatal("friends without a server should fail")
	}
	st.SetSetting(keyServerID, "abc")
	st.SetSecretSetting(keyAdminToken, "admin")
	fr, err := l.Friends(ctx)
	if err != nil || len(fr) != 1 || fr[0].PlexID != 11 || fr[0].Username != "eryn" {
		t.Fatalf("friends: %+v %v", fr, err)
	}
	if ok, _ := l.HasAccess(ctx, "friend"); !ok {
		t.Fatal("friend should have access")
	}
	if ok, _ := l.HasAccess(ctx, "stranger"); ok {
		t.Fatal("stranger shouldn't have access")
	}
}
