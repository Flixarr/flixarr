package server

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/Flixarr/flixarr/internal/requests"
	"github.com/Flixarr/flixarr/internal/secret"
	"github.com/Flixarr/flixarr/internal/store"
)

func TestHideBlocked(t *testing.T) {
	dir := t.TempDir()
	box, _ := secret.Load(dir)
	st, _ := store.Open(dir, box)
	t.Cleanup(func() { st.Close() })
	s := &Server{Store: st}
	admin, _ := st.CreateUser(store.User{PlexID: 1, Username: "admin", Role: "Admin"})
	user, _ := st.CreateUser(store.User{PlexID: 2, Username: "eryn", Role: "User"})
	st.Block(store.Blocked{Type: "movie", TmdbID: 603, Title: "The Matrix"}, admin.ID)
	body := []byte(`{"id":604,"type":"movie","rating":7.1,"similar":[{"id":603,"type":"movie"},{"id":603,"type":"series"},{"id":605,"type":"movie"}]}`)
	if got := string(s.hideBlocked(body, user)); got != string(body) {
		t.Fatalf("setting off: %s", got)
	}
	st.SetSetting(KeyHideBlocked, "true")
	got := string(s.hideBlocked(body, user))
	if strings.Contains(got, `{"id":603,"type":"movie"}`) || !strings.Contains(got, `"id":603,"type":"series"`) || !strings.Contains(got, `"rating":7.1`) {
		t.Fatalf("hidden for a user: %s", got)
	}
	if got := string(s.hideBlocked(body, admin)); got != string(body) {
		t.Fatalf("admins still see it: %s", got)
	}
}

func TestTitleDelete(t *testing.T) {
	dir := t.TempDir()
	box, _ := secret.Load(dir)
	st, _ := store.Open(dir, box)
	t.Cleanup(func() { st.Close() })
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := &Server{Store: st, Log: log, Requests: &requests.Service{Store: st, Log: log}}
	admin, _ := st.CreateUser(store.User{PlexID: 1, Username: "admin", Role: "Admin"})
	mod, _ := st.CreateUser(store.User{PlexID: 2, Username: "mod", Perms: []string{"manageRequests"}})
	st.CreateRequest(store.Request{Type: "movie", TmdbID: 603, Title: "The Matrix", UserID: admin.ID, Status: "available"})
	call := func(u *store.User, body string) int {
		r := httptest.NewRequest("POST", "/api/titles/movie/603/delete", strings.NewReader(body))
		r.SetPathValue("type", "movie")
		r.SetPathValue("id", "603")
		w := httptest.NewRecorder()
		s.titleDelete(w, r, u)
		return w.Code
	}
	if c := call(admin, `{}`); c != 400 {
		t.Fatalf("nothing ticked: %d", c)
	}
	if c := call(mod, `{"files":true}`); c != 403 {
		t.Fatalf("files without Redownload: %d", c)
	}
	if c := call(mod, `{"data":true}`); c != 200 {
		t.Fatalf("data only: %d", c)
	}
	if got, _ := st.RequestsForTitle("movie", 603); len(got) != 0 {
		t.Fatal("requests left")
	}
}

func TestRequestDetailForEveryone(t *testing.T) {
	dir := t.TempDir()
	box, _ := secret.Load(dir)
	st, _ := store.Open(dir, box)
	t.Cleanup(func() { st.Close() })
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := &Server{Store: st, Log: log, Requests: &requests.Service{Store: st, Log: log}}
	admin, _ := st.CreateUser(store.User{PlexID: 1, Username: "admin", Role: "Admin"})
	user, _ := st.CreateUser(store.User{PlexID: 2, Username: "eryn", Perms: []string{"requestMovies"}})
	req, _ := st.CreateRequest(store.Request{Type: "movie", TmdbID: 603, Title: "The Matrix", UserID: admin.ID, Status: "failed", Error: "Radarr said no"})
	r := httptest.NewRequest("GET", "/api/requests/1/detail", nil)
	r.SetPathValue("id", strconv.FormatInt(req.ID, 10))
	w := httptest.NewRecorder()
	s.requestDetail(w, r, user)
	var d requests.Detail
	json.Unmarshal(w.Body.Bytes(), &d)
	if w.Code != 200 || d.Request.ID != req.ID || d.Request.Detail != "" || d.Live.Link != "" || strings.Contains(d.Request.Error, "Radarr") {
		t.Fatalf("someone else's request, as a user: %d %+v", w.Code, d.Request)
	}
}
