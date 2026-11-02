package server

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Flixarr/flixarr/internal/secret"
	"github.com/Flixarr/flixarr/internal/store"
)

// Admins change the What's wrong list (#194): it's checked, the report window follows it, a reason with no
// details needs a note, and Reset puts the defaults back.
func TestIssueKinds(t *testing.T) {
	dir := t.TempDir()
	box, _ := secret.Load(dir)
	st, _ := store.Open(dir, box)
	t.Cleanup(func() { st.Close() })
	s := &Server{Store: st}
	admin, _ := st.CreateUser(store.User{PlexID: 1, Username: "admin", Role: "Admin"})
	put := func(body string) (int, string) {
		w := httptest.NewRecorder()
		s.issueKindsSettings(w, httptest.NewRequest("PUT", "/api/settings/issues", strings.NewReader(body)), admin)
		return w.Code, w.Body.String()
	}
	for _, bad := range []string{
		`{"kinds":[{"kind":"Video","subs":[]}]}`,           // Didn't download removed
		`{"kinds":[{"kind":"Didn't download","subs":[]}]}`, // nothing else left
		`{"kinds":[{"kind":"Video","subs":[]},{"kind":" video ","subs":[]},{"kind":"Didn't download","subs":[]}]}`,
		`{"kinds":[{"kind":"Video","subs":["Bad"," bad"]},{"kind":"Didn't download","subs":[]}]}`,
		`{"kinds":[{"kind":"","subs":[]},{"kind":"Didn't download","subs":[]}]}`,
	} {
		if c, _ := put(bad); c != 400 {
			t.Fatalf("accepted %s", bad)
		}
	}
	if c, b := put(`{"kinds":[{"kind":" Picture ","subs":[" Too dark","Wrong file"]},{"kind":"Didn't download","subs":["Never started"]},{"kind":"Something else","subs":[]}]}`); c != 200 || !strings.Contains(b, `"custom":true`) {
		t.Fatalf("save: %d %s", c, b)
	}
	got := s.issueKinds()
	if len(got) != 3 || got[0].Kind != "Picture" || got[0].Subs[0] != "Too dark" || got[2].Kind != "Something else" {
		t.Fatalf("saved list: %+v", got)
	}
	m := issueKindMap(got)
	if _, ok := m["Video"]; ok {
		t.Fatal("removed reasons can't be picked")
	}
	if subs, ok := m["Something else"]; !ok || len(subs) != 0 {
		t.Fatalf("no-detail reason: %+v", m)
	}
	if c, b := put(`{"reset":true}`); c != 200 || !strings.Contains(b, `"custom":false`) {
		t.Fatalf("reset: %d %s", c, b)
	}
	var back struct{ Kinds []IssueKind }
	w := httptest.NewRecorder()
	s.issueKindsRead(w, httptest.NewRequest("GET", "/api/issues/kinds", nil), admin)
	json.NewDecoder(w.Body).Decode(&back)
	if len(back.Kinds) != len(defaultIssueKinds) || back.Kinds[0].Kind != "Video" {
		t.Fatalf("after reset: %+v", back.Kinds)
	}
}
