package server

import (
	"io"
	"log/slog"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/Flixarr/flixarr/internal/secret"
	"github.com/Flixarr/flixarr/internal/store"
)

// Managers change people up to Manager; never the admin switch, admins or the owner (#148).
func TestManagerUserUpdate(t *testing.T) {
	dir := t.TempDir()
	box, _ := secret.Load(dir)
	st, _ := store.Open(dir, box)
	t.Cleanup(func() { st.Close() })
	s := &Server{Store: st}
	owner, _ := st.CreateUser(store.User{PlexID: 1, Username: "owner", Role: "Admin"})
	other, _ := st.CreateUser(store.User{PlexID: 2, Username: "other admin", Role: "Admin"})
	mgr, _ := st.CreateUser(store.User{PlexID: 3, Username: "mgr", Role: "Manager"})
	user, _ := st.CreateUser(store.User{PlexID: 4, Username: "eryn", Role: "User"})
	trusted, _ := st.CreateUser(store.User{PlexID: 5, Username: "jace", Role: "Trusted"})
	if !mgr.CanManageUsers() || user.CanManageUsers() || trusted.CanManageUsers() || !owner.CanManageUsers() {
		t.Fatal("who manages users")
	}
	put := func(me *store.User, id int64, body string) int {
		r := httptest.NewRequest("PUT", "/api/users/"+strconv.FormatInt(id, 10), strings.NewReader(body))
		r.SetPathValue("id", strconv.FormatInt(id, 10))
		w := httptest.NewRecorder()
		s.people(s.userUpdate)(w, r, me)
		return w.Code
	}
	manager := `{"perms":["manageRequests","autoApprove","requestMovies","requestTv","advanced","reportIssues","manageIssues","redownload","manageUsers","watchlist"],"movieLimit":0,"seasonLimit":0}`
	if c := put(mgr, user.ID, manager); c != 200 {
		t.Fatalf("manager makes a manager: %d", c)
	}
	if u, _ := st.UserByID(user.ID); u.Role != "Manager" || !u.Can("watchlist") {
		t.Fatalf("not saved: %+v", u)
	}
	if c := put(mgr, trusted.ID, `{"perms":["admin"],"movieLimit":0,"seasonLimit":0}`); c != 403 {
		t.Fatalf("manager gives admin: %d", c)
	}
	if c := put(mgr, other.ID, `{"perms":["requestMovies"],"movieLimit":0,"seasonLimit":0}`); c != 403 {
		t.Fatalf("manager changes an admin: %d", c)
	}
	if c := put(mgr, owner.ID, `{"perms":["requestMovies"],"movieLimit":0,"seasonLimit":0}`); c != 403 {
		t.Fatalf("manager changes the owner: %d", c)
	}
	if c := put(trusted, user.ID, `{"perms":["requestMovies"],"movieLimit":0,"seasonLimit":0}`); c != 403 {
		t.Fatalf("trusted changes someone: %d", c)
	}
	if c := put(owner, other.ID, `{"perms":["requestMovies"],"movieLimit":0,"seasonLimit":0}`); c != 200 {
		t.Fatalf("admin changes an admin: %d", c)
	}
	if c := put(owner, owner.ID, `{"perms":["requestMovies"],"movieLimit":0,"seasonLimit":0}`); c != 409 {
		t.Fatalf("admin drops own admin: %d", c)
	}
}

// Many at once (#147): a role keeps manual switches, set changes only what it names, and people the
// signed-in person may not change are skipped with why.
func TestUserBulk(t *testing.T) {
	dir := t.TempDir()
	box, _ := secret.Load(dir)
	st, _ := store.Open(dir, box)
	t.Cleanup(func() { st.Close() })
	s := &Server{Store: st, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	owner, _ := st.CreateUser(store.User{PlexID: 1, Username: "owner", Role: "Admin"})
	mgr, _ := st.CreateUser(store.User{PlexID: 2, Username: "mgr", Role: "Manager"})
	a, _ := st.CreateUser(store.User{PlexID: 3, Username: "a", Perms: []string{"autoApprove", "requestMovies", "watchlist"}})
	b, _ := st.CreateUser(store.User{PlexID: 4, Username: "b", Role: "Trusted"})
	post := func(me *store.User, body string) (int, string) {
		r := httptest.NewRequest("POST", "/api/users/bulk", strings.NewReader(body))
		w := httptest.NewRecorder()
		s.people(s.userBulk)(w, r, me)
		return w.Code, w.Body.String()
	}
	ids := `[` + strconv.FormatInt(owner.ID, 10) + `,` + strconv.FormatInt(a.ID, 10) + `,` + strconv.FormatInt(b.ID, 10) + `]`
	code, body := post(mgr, `{"ids":`+ids+`,"role":"User"}`)
	if code != 200 || !strings.Contains(body, `"updated":2`) || !strings.Contains(body, `"name":"owner"`) {
		t.Fatalf("role: %d %s", code, body)
	}
	if u, _ := st.UserByID(a.ID); u.Role != "User" || !u.Can("watchlist") || u.Can("autoApprove") {
		t.Fatalf("a: %+v", u.Perms)
	}
	// Only the named switch changes.
	post(owner, `{"ids":`+ids+`,"set":{"redownload":true}}`)
	if u, _ := st.UserByID(b.ID); !u.Can("redownload") || u.Can("autoApprove") || !u.Can("requestMovies") {
		t.Fatalf("b: %+v", u.Perms)
	}
	if u, _ := st.UserByID(owner.ID); !u.Can("admin") {
		t.Fatal("owner lost admin")
	}
	// A Manager can't give admin: everyone skipped.
	if _, body := post(mgr, `{"ids":`+ids+`,"set":{"admin":true}}`); !strings.Contains(body, `"updated":0`) {
		t.Fatalf("manager gave admin: %s", body)
	}
}
