package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"

	"github.com/Flixarr/flixarr/internal/store"
)

// KeyPlexSignIn: "true" = anyone the Plex server is shared with can sign in (as a User).
const KeyPlexSignIn = "users_plex_signin"

func (s *Server) plexSignInOpen() bool { return s.Store.Setting(KeyPlexSignIn, "true") == "true" }

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

func (s *Server) userList(w http.ResponseWriter, r *http.Request, _ *store.User) {
	list, err := s.Store.Users()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Database error.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": list, "perms": store.AllPerms, "roles": store.RolePerms, "plexSignIn": s.plexSignInOpen()})
}

func (s *Server) userUpdate(w http.ResponseWriter, r *http.Request, me *store.User) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var in struct {
		Perms []string `json:"perms"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in); err != nil || in.Perms == nil {
		writeErr(w, http.StatusBadRequest, "Bad request.")
		return
	}
	if id == me.ID && !slices.Contains(in.Perms, "admin") {
		writeErr(w, http.StatusConflict, "You can't take admin away from yourself.")
		return
	}
	u, err := s.Store.SetPerms(id, in.Perms)
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
		if _, err := s.Store.CreateUser(store.User{PlexID: f.PlexID, Username: f.Username, Email: f.Email, Avatar: f.Thumb, Role: "User"}); err == nil {
			added++
		}
	}
	s.Log.Info("plex users imported", "added", added)
	writeJSON(w, http.StatusOK, map[string]int{"added": added})
}

func (s *Server) userSettings(w http.ResponseWriter, r *http.Request, _ *store.User) {
	if r.Method == http.MethodPut {
		var in struct {
			PlexSignIn bool `json:"plexSignIn"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&in); err != nil {
			writeErr(w, http.StatusBadRequest, "Bad request.")
			return
		}
		s.Store.SetSetting(KeyPlexSignIn, strconv.FormatBool(in.PlexSignIn))
	}
	writeJSON(w, http.StatusOK, map[string]bool{"plexSignIn": s.plexSignInOpen()})
}
