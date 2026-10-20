package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"

	"github.com/Flixarr/flixarr/internal/notify"
	"github.com/Flixarr/flixarr/internal/store"
)

// KeyPlexSignIn: "true" = anyone the Plex server is shared with can sign in (as a User).
const KeyPlexSignIn = "users_plex_signin"

// KeySignIn is how people sign in: "both" (default), "plex" or "local" (username and password). With
// "local", admins can still sign in with Plex (the owner's account is a Plex one).
const KeySignIn = "sign_in"

func (s *Server) signIn() string {
	if m := s.Store.Setting(KeySignIn, "both"); m == "plex" || m == "local" {
		return m
	}
	return "both"
}

func (s *Server) plexSignInOpen() bool { return s.Store.Setting(KeyPlexSignIn, "true") == "true" }

// KeyDefaultRole is the role new people get (signing in, or imported): User or Trusted.
const KeyDefaultRole = "users_default_role"

func (s *Server) defaultRole() string {
	if r := s.Store.Setting(KeyDefaultRole, "User"); r == "Trusted" {
		return r
	}
	return "User"
}

// allow wraps an authenticated handler so only people with a permission reach it.
func (s *Server) allow(perm string, h func(http.ResponseWriter, *http.Request, *store.User)) func(http.ResponseWriter, *http.Request, *store.User) {
	return func(w http.ResponseWriter, r *http.Request, u *store.User) {
		if !u.Can(perm) {
			writeErr(w, http.StatusForbidden, "You don't have permission to do that.")
			return
		}
		h(w, r, u)
	}
}

// ---------- Users page ----------

func (s *Server) userList(w http.ResponseWriter, r *http.Request, me *store.User) {
	list, err := s.Store.Users()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Database error.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": list, "perms": store.AllPerms, "roles": store.RolePerms, "plexSignIn": s.plexSignInOpen(), "admin": me.Can("admin")})
}

func (s *Server) userUpdate(w http.ResponseWriter, r *http.Request, me *store.User) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var in struct {
		Perms       []string `json:"perms"`
		MovieLimit  int      `json:"movieLimit"`
		SeasonLimit int      `json:"seasonLimit"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in); err != nil || in.Perms == nil {
		writeErr(w, http.StatusBadRequest, "Bad request.")
		return
	}
	if !me.Can("admin") {
		// Managers: up to Manager, never the admin switch, admins or the owner (#148).
		if slices.Contains(in.Perms, "admin") {
			writeErr(w, http.StatusForbidden, "Only admins can give admin.")
			return
		}
		if t, err := s.Store.UserByID(id); err == nil && (t.Owner || t.Can("admin")) {
			writeErr(w, http.StatusForbidden, "Only admins can change an admin.")
			return
		}
	} else if id == me.ID && !slices.Contains(in.Perms, "admin") {
		writeErr(w, http.StatusConflict, "You can't take admin away from yourself.")
		return
	}
	if in.MovieLimit < 0 || in.SeasonLimit < 0 || in.MovieLimit > 1000 || in.SeasonLimit > 1000 {
		writeErr(w, http.StatusBadRequest, "Limits go from 0 (unlimited) to 1000.")
		return
	}
	u, err := s.Store.SetPerms(id, in.Perms)
	if err == nil {
		s.Store.SetLimits(id, in.MovieLimit, in.SeasonLimit)
		u, err = s.Store.UserByID(id)
	}
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, http.StatusNotFound, "That user doesn't exist.")
	case errors.Is(err, store.ErrOwner):
		writeErr(w, http.StatusConflict, "The owner always stays an admin.")
	case err != nil:
		writeErr(w, http.StatusInternalServerError, "Database error.")
	default:
		writeJSON(w, http.StatusOK, u)
	}
}

func (s *Server) userDelete(w http.ResponseWriter, r *http.Request, me *store.User) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if id == me.ID {
		writeErr(w, http.StatusConflict, "You can't remove yourself.")
		return
	}
	err := s.Store.DeleteUser(id)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, http.StatusNotFound, "That user doesn't exist.")
	case errors.Is(err, store.ErrOwner):
		writeErr(w, http.StatusConflict, "The owner can't be removed.")
	case err != nil:
		writeErr(w, http.StatusInternalServerError, "Database error.")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// userPlexList: the people the Plex server is shared with, and whether each is already a user.
func (s *Server) userPlexList(w http.ResponseWriter, r *http.Request, _ *store.User) {
	friends, err := s.Library.Friends(r.Context())
	if err != nil {
		writeErr(w, http.StatusBadGateway, capitalize(err.Error())+".")
		return
	}
	type row struct {
		PlexID   int64  `json:"plexId"`
		Username string `json:"username"`
		Email    string `json:"email"`
		Avatar   string `json:"avatar"`
		Added    bool   `json:"added"`
	}
	out := []row{}
	for _, f := range friends {
		_, err := s.Store.UserByPlexID(f.PlexID)
		out = append(out, row{f.PlexID, f.Username, f.Email, f.Thumb, err == nil})
	}
	writeJSON(w, http.StatusOK, out)
}

// userPlexImport adds the chosen Plex friends as Users. Add-only: existing users are left alone.
func (s *Server) userPlexImport(w http.ResponseWriter, r *http.Request, _ *store.User) {
	var in struct {
		PlexIDs []int64 `json:"plexIds"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 65536)).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "Bad request.")
		return
	}
	friends, err := s.Library.Friends(r.Context())
	if err != nil {
		writeErr(w, http.StatusBadGateway, capitalize(err.Error())+".")
		return
	}
	added := 0
	for _, f := range friends {
		if !slices.Contains(in.PlexIDs, f.PlexID) {
			continue
		}
		if _, err := s.Store.UserByPlexID(f.PlexID); err == nil {
			continue
		}
		if _, err := s.Store.CreateUser(store.User{PlexID: f.PlexID, Username: f.Username, Email: f.Email, Avatar: f.Thumb, Role: s.defaultRole()}); err == nil {
			added++
		}
	}
	s.Log.Info("plex users imported", "added", added)
	writeJSON(w, http.StatusOK, map[string]int{"added": added})
}

func (s *Server) userSettings(w http.ResponseWriter, r *http.Request, _ *store.User) {
	if r.Method == http.MethodPut {
		var in struct {
			PlexSignIn  bool   `json:"plexSignIn"`
			DefaultRole string `json:"defaultRole"`
			SignIn      string `json:"signIn"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&in); err != nil {
			writeErr(w, http.StatusBadRequest, "Bad request.")
			return
		}
		s.Store.SetSetting(KeyPlexSignIn, strconv.FormatBool(in.PlexSignIn))
		if in.SignIn == "both" || in.SignIn == "plex" || in.SignIn == "local" {
			s.Store.SetSetting(KeySignIn, in.SignIn)
		}
		if in.DefaultRole == "User" || in.DefaultRole == "Trusted" {
			s.Store.SetSetting(KeyDefaultRole, in.DefaultRole)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"plexSignIn": s.plexSignInOpen(), "defaultRole": s.defaultRole(), "signIn": s.signIn()})
}

// myWatchlist reads or changes the Profile's "Request from my Plex Watchlist" switch.
// myNotifications is Profile → Notifications: which events reach this person by email and push.
// Channels show only when an admin has set them up (and email only with an address on the account).
func (s *Server) myNotifications(w http.ResponseWriter, r *http.Request, u *store.User) {
	if r.Method == http.MethodPut {
		var in struct {
			Off map[string][]string `json:"off"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&in); err != nil {
			writeErr(w, http.StatusBadRequest, "Bad request.")
			return
		}
		if err := s.Notify.SetOff(u.ID, in.Off); err != nil {
			writeErr(w, http.StatusInternalServerError, "Database error.")
			return
		}
	}
	emailOn, emailSent := s.Notify.Service("email")
	pushOn, pushSent := s.Notify.Service("push")
	writeJSON(w, http.StatusOK, map[string]any{
		"channels": map[string]bool{"email": emailOn && u.Email != "", "push": pushOn},
		"events":   s.withoutMaintainerr(notify.EventsFor(u)),
		"sent":     map[string][]string{"email": emailSent, "push": pushSent}, // what Settings sends at all
		"off":      s.Notify.Off(u.ID),
	})
}

func (s *Server) myWatchlist(w http.ResponseWriter, r *http.Request, u *store.User) {
	if r.Method == http.MethodPut {
		var in struct {
			On bool `json:"on"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&in); err != nil {
			writeErr(w, http.StatusBadRequest, "Bad request.")
			return
		}
		if in.On && !u.Can("watchlist") {
			writeErr(w, http.StatusForbidden, "You don't have permission to request from your Plex Watchlist.")
			return
		}
		s.Store.SetWatchlist(u.ID, in.On)
		if in.On {
			s.Jobs.RunNow(s.BaseCtx, "watchlist")
		}
	}
	tokenOK := s.Store.PlexToken(u.ID) != "" || (u.Owner && s.Store.SecretSetting("plex_admin_token", "") != "")
	writeJSON(w, http.StatusOK, map[string]bool{"on": s.Store.Watchlist(u.ID), "allowed": u.Can("watchlist"), "signedIn": tokenOK})
}
