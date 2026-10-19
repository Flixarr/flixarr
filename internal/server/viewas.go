package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/Flixarr/flixarr/internal/store"
	"github.com/Flixarr/flixarr/internal/testinstall"
)

// View as (test installs only): an admin sees and uses the app with the permissions of another role, for
// real on the server, so every page, button and answer is what that role gets. Who they are doesn't change
// (requests and comments are still theirs), and nothing that changes a person (users, passwords, their own
// settings) can be done while viewing as someone else.

const viewAsCookie = "flixarr_view_as"

// viewAs returns the user as the request should see them.
func viewAs(r *http.Request, u *store.User) (*store.User, error) {
	u.RealAdmin = u.Can("admin")
	if !testinstall.On || !u.RealAdmin {
		return u, nil
	}
	c, err := r.Cookie(viewAsCookie)
	if err != nil {
		return u, nil
	}
	perms, ok := store.RolePerms[c.Value]
	if !ok {
		return u, nil
	}
	v := *u
	v.Perms, v.Role, v.ViewAs, v.Owner = slices.Clone(perms), c.Value, c.Value, false
	if r.Method != http.MethodGet && (strings.HasPrefix(r.URL.Path, "/api/users") || strings.HasPrefix(r.URL.Path, "/api/me/")) {
		return nil, errors.New("leave View as first: people can't be changed while viewing as someone else")
	}
	return &v, nil
}

// viewAsSet switches View as: {"role": "User"}, or "" to be yourself again.
func (s *Server) viewAsSet(w http.ResponseWriter, r *http.Request, u *store.User) {
	if !testinstall.On || !u.RealAdmin {
		writeErr(w, http.StatusForbidden, "View as is only on dev installs, for admins.")
		return
	}
	var in struct {
		Role string `json:"role"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "Bad request.")
		return
	}
	if _, ok := store.RolePerms[in.Role]; in.Role != "" && !ok {
		writeErr(w, http.StatusBadRequest, "Pick Admin, Trusted or User.")
		return
	}
	c := &http.Cookie{Name: viewAsCookie, Value: in.Role, Path: "/", HttpOnly: true, Secure: secure(r), SameSite: http.SameSiteLaxMode}
	if in.Role == "" {
		c.MaxAge = -1
	}
	http.SetCookie(w, c)
	w.WriteHeader(http.StatusNoContent)
}
