package server

import (
	"strings"
	"testing"

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
