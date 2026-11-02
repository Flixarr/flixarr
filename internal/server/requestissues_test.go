package server

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/Flixarr/flixarr/internal/requests"
	"github.com/Flixarr/flixarr/internal/secret"
	"github.com/Flixarr/flixarr/internal/store"
)

// The Requests page and the request window carry the title's open issues; the note only reaches people
// who manage issues and the person who reported it.
func TestRequestIssues(t *testing.T) {
	dir := t.TempDir()
	box, _ := secret.Load(dir)
	st, _ := store.Open(dir, box)
	t.Cleanup(func() { st.Close() })
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := &Server{Store: st, Log: log, Requests: &requests.Service{Store: st, Log: log}}
	admin, _ := st.CreateUser(store.User{PlexID: 1, Username: "admin", Role: "Admin"})
	sam, _ := st.CreateUser(store.User{PlexID: 2, Username: "sam", Role: "User"})
	jo, _ := st.CreateUser(store.User{PlexID: 3, Username: "jo", Role: "User"})
	req, _ := st.CreateRequest(store.Request{Type: "series", TmdbID: 2316, Title: "The Office", UserID: jo.ID, Status: "available"})
	st.CreateIssue(store.Issue{Type: "series", TmdbID: 2316, Title: "The Office", Kind: "Audio", Sub: "Too quiet", Season: 3, Episode: 5, Note: "Can barely hear it", UserID: sam.ID})
	done, _ := st.CreateIssue(store.Issue{Type: "series", TmdbID: 2316, Title: "The Office", Kind: "Video", UserID: sam.ID})
	st.SetIssueStatus(done.ID, admin.ID, "resolved")
	st.CreateIssue(store.Issue{Type: "movie", TmdbID: 2316, Title: "Someone else", Kind: "Video", UserID: sam.ID})

	list := func(u *store.User) map[string][]store.Issue {
		w := httptest.NewRecorder()
		s.requestList(w, httptest.NewRequest("GET", "/api/requests", nil), u)
		var out struct {
			Issues map[string][]store.Issue `json:"issues"`
		}
		json.NewDecoder(w.Body).Decode(&out)
		return out.Issues
	}
	for _, c := range []struct {
		who  *store.User
		note string
	}{{admin, "Can barely hear it"}, {sam, "Can barely hear it"}, {jo, ""}} {
		got := list(c.who)["series:2316"]
		if len(got) != 1 || got[0].Kind != "Audio" || got[0].Season != 3 || got[0].Note != c.note {
			t.Fatalf("list for %s: %+v", c.who.Username, got)
		}
	}
	if got := list(admin)["movie:2316"]; len(got) != 1 {
		t.Fatalf("the movie with the same number is its own title: %+v", got)
	}

	r := httptest.NewRequest("GET", "/api/requests/x/detail", nil)
	r.SetPathValue("id", strconv.FormatInt(req.ID, 10))
	w := httptest.NewRecorder()
	s.requestDetail(w, r, jo)
	var d struct {
		Request store.Request `json:"request"`
		Issues  []store.Issue `json:"issues"`
	}
	json.NewDecoder(w.Body).Decode(&d)
	if d.Request.ID != req.ID || len(d.Issues) != 1 || d.Issues[0].Kind != "Audio" || d.Issues[0].Note != "" {
		t.Fatalf("detail: %d %+v", w.Code, d)
	}
}
