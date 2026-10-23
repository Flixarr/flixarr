package server

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/Flixarr/flixarr/internal/store"
	"github.com/Flixarr/flixarr/internal/testinstall"
)

func TestViewAs(t *testing.T) {
	on := testinstall.On
	t.Cleanup(func() { testinstall.On = on })
	admin := func() *store.User {
		return &store.User{ID: 1, Username: "owner", Perms: slices.Clone(store.AllPerms), Owner: true}
	}
	get := func(method, path, role string) (*store.User, error) {
		r := httptest.NewRequest(method, path, nil)
		if role != "" {
			r.AddCookie(&http.Cookie{Name: viewAsCookie, Value: role})
		}
		return viewAs(r, admin())
	}

	// Not a test install: the cookie is ignored.
	testinstall.On = false
	if u, _ := get("GET", "/api/requests", "User"); u.ViewAs != "" || !u.Can("admin") {
		t.Fatalf("view as outside a test install: %+v", u)
	}

	testinstall.On = true
	u, err := get("GET", "/api/requests", "User")
	if err != nil || u.ViewAs != "User" || u.Can("admin") || u.Owner || u.ID != 1 || !u.RealAdmin {
		t.Fatalf("view as User: %+v %v", u, err)
	}
	if u, _ := get("GET", "/api/requests", "Trusted"); !u.Can("autoApprove") || u.Can("manageRequests") {
		t.Fatalf("view as Trusted: %+v", u.Perms)
	}
	if u, _ := get("GET", "/api/requests", "Admin"); !u.Can("admin") || u.Owner {
		t.Fatalf("view as Admin: %+v", u)
	}
	if u, _ := get("GET", "/api/requests", "Nonsense"); u.ViewAs != "" {
		t.Fatal("unknown role used")
	}
	// Nothing that changes a person while viewing as someone else.
	for _, p := range []string{"/api/users/2", "/api/me/locale"} {
		if _, err := get("PUT", p, "User"); err == nil {
			t.Errorf("%s allowed while viewing as User", p)
		}
	}
	if _, err := get("POST", "/api/requests", "User"); err != nil {
		t.Errorf("requests blocked: %v", err)
	}
	// Someone who isn't an admin can't view as anyone.
	r := httptest.NewRequest("GET", "/api/requests", nil)
	r.AddCookie(&http.Cookie{Name: viewAsCookie, Value: "Admin"})
	if u, _ := viewAs(r, &store.User{ID: 5, Perms: []string{"requestMovies"}}); u.Can("admin") || u.ViewAs != "" {
		t.Fatal("a user made themselves admin")
	}
}
