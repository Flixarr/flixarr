package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"

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
	writeJSON(w, http.StatusOK, map[string]any{"users": list, "perms": store.AllPerms, "roles": store.RolePerms, "plexSignIn": s.plexSignInOpen(), "admin": me.Can("admin"),
		"approvalRequired": s.Requests.ApprovalRequired()})
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
	if t, err := s.Store.UserByID(id); err == nil {
		if code, why := mayChange(me, t, in.Perms); code != 0 {
			writeErr(w, code, why)
			return
		}
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

// mayChange says whether me may give t these switches: 0 when yes, otherwise a status and why not.
// Managers change people up to Manager: never the admin switch, admins or the owner (#148). Nobody takes
// admin away from themselves or the owner.
func mayChange(me, t *store.User, perms []string) (int, string) {
	admin := slices.Contains(perms, "admin")
	if !me.Can("admin") {
		if t.Owner || t.Can("admin") {
			return http.StatusForbidden, "Only admins can change an admin."
		}
		if admin {
			return http.StatusForbidden, "Only admins can give admin."
		}
	}
	if t.Owner && !admin {
		return http.StatusConflict, "The owner always stays an admin."
	}
	if t.ID == me.ID && me.Can("admin") && !admin {
		return http.StatusConflict, "You can't take admin away from yourself."
	}
	return 0, ""
}

// userProfile is a person's profile page (#149): who they are, their numbers and their latest requests.
func (s *Server) userProfile(w http.ResponseWriter, r *http.Request, me *store.User) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	u, err := s.Store.UserByID(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "That user doesn't exist.")
		return
	}
	u.LastSeen = s.Store.LastSeen(id)
	stats, _ := s.userStats(id)
	cards, _ := s.requestCards(r, id)
	writeJSON(w, http.StatusOK, map[string]any{"user": u, "stats": stats, "requests": cards, "roles": store.RolePerms,
		"admin": me.Can("admin"), "canChange": me.Can("admin") || !(u.Owner || u.Can("admin")), "approvalRequired": s.Requests.ApprovalRequired()})
}

// userBulk changes many people at once (#147): {ids, role, set}. role puts them on a preset (their manual
// switches stay); set turns switches on (true) or off (false); switches not in set are left alone. People
// the signed-in person may not change are skipped, with why.
func (s *Server) userBulk(w http.ResponseWriter, r *http.Request, me *store.User) {
	var in struct {
		IDs  []int64         `json:"ids"`
		Role string          `json:"role"`
		Set  map[string]bool `json:"set"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 65536)).Decode(&in); err != nil || len(in.IDs) == 0 || len(in.IDs) > 10000 {
		writeErr(w, http.StatusBadRequest, "Bad request.")
		return
	}
	preset, isRole := store.RolePerms[in.Role]
	if in.Role != "" && !isRole {
		writeErr(w, http.StatusBadRequest, "Pick User, Trusted or Manager.")
		return
	}
	type skip struct {
		Name string `json:"name"`
		Why  string `json:"why"`
	}
	updated, skipped := 0, []skip{}
	for _, id := range in.IDs {
		t, err := s.Store.UserByID(id)
		if err != nil {
			continue
		}
		perms := slices.Clone(t.Perms)
		if isRole {
			perms = slices.Clone(preset)
			for _, p := range store.ManualPerms {
				if slices.Contains(t.Perms, p) {
					perms = append(perms, p)
				}
			}
		}
		for p, on := range in.Set {
			perms = slices.DeleteFunc(perms, func(x string) bool { return x == p })
			if on {
				perms = append(perms, p)
			}
		}
		if code, why := mayChange(me, t, perms); code != 0 {
			skipped = append(skipped, skip{t.Username, strings.TrimSuffix(why, ".")})
			continue
		}
		if _, err := s.Store.SetPerms(id, perms); err != nil {
			skipped = append(skipped, skip{t.Username, "couldn't be saved"})
			continue
		}
		updated++
	}
	s.Log.Info("users changed together", "by", me.Username, "updated", updated, "skipped", len(skipped))
	writeJSON(w, http.StatusOK, map[string]any{"updated": updated, "skipped": skipped})
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
			Off      map[string][]string `json:"off"`
			Accounts map[string]string   `json:"accounts"` // optional: own accounts on Discord, Telegram, Pushover, ntfy
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&in); err != nil {
			writeErr(w, http.StatusBadRequest, "Bad request.")
			return
		}
		if in.Accounts != nil {
			if err := s.Notify.SetAccounts(u.ID, in.Accounts); err != nil {
				writeErr(w, http.StatusBadRequest, capitalize(err.Error())+".")
				return
			}
		}
		if in.Off != nil {
			if err := s.Notify.SetOff(u.ID, in.Off); err != nil {
				writeErr(w, http.StatusForbidden, capitalize(err.Error())+".")
				return
			}
		}
	}
	// A personal channel shows when the server sends through that service, and (for the account services)
	// once the person added their own account there (#124).
	accounts := s.Notify.Accounts(u.ID)
	channels, sent, services := map[string]bool{}, map[string][]string{}, map[string]bool{}
	for _, ch := range notify.PersonalChannels {
		on, events := s.Notify.Service(ch)
		sent[ch] = events // what Settings sends at all
		switch ch {
		case "email":
			channels[ch] = on && u.Email != ""
		case "push":
			channels[ch] = on
		default:
			services[ch] = on
			channels[ch] = on && accounts[ch] != ""
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"channels": channels,
		"services": services, // account services an admin set up: Profile asks for the person's account there
		"accounts": accounts,
		"events":   s.withoutMaintainerr(notify.EventsFor(u)),
		"sent":     sent,
		"off":      s.Notify.Off(u.ID),
		"locked":   s.Notify.OffLocked(), // an admin set them for everyone (#110)
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
		if in.On && s.Store.Setting(store.KeyWatchlistOff, "") == "true" {
			writeErr(w, http.StatusForbidden, "An admin turned Plex Watchlist requests off for everyone.")
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
	off := s.Store.Setting(store.KeyWatchlistOff, "") == "true"
	writeJSON(w, http.StatusOK, map[string]bool{"on": s.Store.Watchlist(u.ID) && !off, "allowed": u.Can("watchlist"), "signedIn": tokenOK, "locked": off})
}

// myDevices lists the devices getting this person's push notifications (Profile; remove one with DELETE /api/push).
func (s *Server) myDevices(w http.ResponseWriter, r *http.Request, u *store.User) {
	list, err := s.Store.Devices(u.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Database error.")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// everyoneSettings is Settings → Users → Everyone's own settings (#110): Plex Watchlist requests off for
// everyone, everyone on the server's region and language, and the notifications people start with (and
// whether they can change them). PUT saves; POST .../apply resets everyone's own notification choices.
func (s *Server) everyoneSettings(w http.ResponseWriter, r *http.Request, _ *store.User) {
	if r.Method == http.MethodPut {
		var in struct {
			WatchlistOff bool                `json:"watchlistOff"`
			LocaleLocked bool                `json:"localeLocked"`
			DefaultOff   map[string][]string `json:"defaultOff"`
			OffLocked    bool                `json:"offLocked"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384)).Decode(&in); err != nil {
			writeErr(w, http.StatusBadRequest, "Bad request.")
			return
		}
		s.Store.SetSetting(store.KeyWatchlistOff, strconv.FormatBool(in.WatchlistOff))
		s.Store.SetSetting(store.KeyLocaleLocked, strconv.FormatBool(in.LocaleLocked))
		if err := s.Notify.SetDefaultOff(in.DefaultOff, in.OffLocked); err != nil {
			writeErr(w, http.StatusInternalServerError, "Database error.")
			return
		}
	}
	def, locked := s.Notify.DefaultOff()
	channels := []string{}
	for _, ch := range notify.PersonalChannels {
		if on, _ := s.Notify.Service(ch); on {
			channels = append(channels, ch)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"watchlistOff": s.Store.Setting(store.KeyWatchlistOff, "") == "true",
		"localeLocked": s.Store.Setting(store.KeyLocaleLocked, "") == "true",
		"defaultOff":   def,
		"offLocked":    locked,
		"channels":     channels, // personal channels this server sends through
		"events":       s.withoutMaintainerr(notify.Events),
	})
}

// everyoneApply puts everyone back on the starting notification choices.
func (s *Server) everyoneApply(w http.ResponseWriter, r *http.Request, u *store.User) {
	if err := s.Notify.ResetEveryone(); err != nil {
		writeErr(w, http.StatusInternalServerError, "Database error.")
		return
	}
	s.Log.Info("everyone's notification choices reset", "by", u.Username)
	w.WriteHeader(http.StatusNoContent)
}
