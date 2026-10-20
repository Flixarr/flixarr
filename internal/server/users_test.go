package server

import (
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
	manager := `{"perms":["manageRequests","autoApprove","requestMovies","requestTv","advanced","reportIssues","manageIssues","redownload","watchlist"],"movieLimit":0,"seasonLimit":0}`
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
