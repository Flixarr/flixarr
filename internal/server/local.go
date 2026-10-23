package server

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Flixarr/flixarr/internal/password"
	"github.com/Flixarr/flixarr/internal/store"
)

// Local accounts: people without Plex sign in with a username and password an admin gave them.

// Too many wrong passwords for one username, or from one address, pause sign-in for that name/address.
const (
	maxFails  = 5
	failSpan  = 15 * time.Minute
	lockedFor = 15 * time.Minute
)

type failures struct {
	mu   sync.Mutex
	seen map[string][]time.Time
}

func (f *failures) locked(keys ...string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, k := range keys {
		recent := 0
		for _, t := range f.seen[k] {
			if time.Since(t) < failSpan {
				recent++
			}
		}
		if recent >= maxFails {
			if last := f.seen[k][len(f.seen[k])-1]; time.Since(last) < lockedFor {
				return true
			}
		}
	}
	return false
}

func (f *failures) add(keys ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.seen == nil {
		f.seen = map[string][]time.Time{}
	}
	for k, list := range f.seen { // forget names and addresses that have gone quiet
		if time.Since(list[len(list)-1]) > failSpan+lockedFor {
			delete(f.seen, k)
		}
	}
	for _, k := range keys {
		list := append(f.seen[k], time.Now())
		if len(list) > maxFails*2 {
			list = list[len(list)-maxFails*2:]
		}
		f.seen[k] = list
	}
}

func (f *failures) clear(key string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.seen, key)
}

// clientIP is the caller's address. A forwarded address is believed only from a reverse proxy on the
// local network (anyone else could make one up to dodge the sign-in pause).
func clientIP(r *http.Request) string {
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	if ip := net.ParseIP(host); ip != nil && (ip.IsLoopback() || ip.IsPrivate()) {
		if f := r.Header.Get("X-Forwarded-For"); f != "" {
			return strings.TrimSpace(strings.Split(f, ",")[0])
		}
	}
	return host
}

// checking limits how many password checks run at once: each is deliberately slow, so a flood of
// sign-ins waits its turn instead of using every CPU.
var checking = make(chan struct{}, 3)

func checkPassword(pw, hash string) bool {
	checking <- struct{}{}
	defer func() { <-checking }()
	return password.Check(pw, hash)
}

func (s *Server) localLogin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in); err != nil || strings.TrimSpace(in.Username) == "" || in.Password == "" {
		writeErr(w, http.StatusBadRequest, "Enter your username and password.")
		return
	}
	if s.signIn() == "plex" {
		writeErr(w, http.StatusForbidden, "Sign in with Plex.")
		return
	}
	name := strings.ToLower(strings.TrimSpace(in.Username))
	byName, byIP := "name:"+name, "ip:"+clientIP(r)
	if s.fails.locked(byName, byIP) {
		writeErr(w, http.StatusTooManyRequests, "Too many wrong passwords. Try again in 15 minutes.")
		return
	}
	u, hash, err := s.Store.LocalLogin(name)
	ok := err == nil && checkPassword(in.Password, hash)
	if !ok && errors.Is(err, store.ErrNotFound) {
		checkPassword(in.Password, dummyHash) // same work either way: a wrong name isn't faster than a wrong password
	}
	if !ok {
		s.fails.add(byName, byIP)
		s.Log.Info("local sign-in refused", "user", name, "from", clientIP(r))
		writeErr(w, http.StatusUnauthorized, "That username and password don't match.")
		return
	}
	s.fails.clear(byName)
	token, err := s.Store.CreateSession(u.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Couldn't start a session.")
		return
	}
	s.setSession(w, r, token)
	writeJSON(w, http.StatusOK, map[string]any{"done": true, "user": u})
}

// dummyHash is checked when the username doesn't exist (any valid hash works).
var dummyHash, _ = password.Hash("not a real password")

// userAddLocal (admin): {username, email, role, password}.
func (s *Server) userAddLocal(w http.ResponseWriter, r *http.Request, _ *store.User) {
	var in struct {
		Username string `json:"username"`
		Email    string `json:"email"`
		Role     string `json:"role"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "Bad request.")
		return
	}
	in.Username, in.Email = strings.TrimSpace(in.Username), strings.TrimSpace(in.Email)
	if in.Username == "" || len(in.Username) > 64 {
		writeErr(w, http.StatusBadRequest, "Enter a username (up to 64 characters).")
		return
	}
	if in.Role != "Trusted" {
		in.Role = "User" // more is given afterwards, on purpose, like anyone else's switches
	}
	hash, err := password.Hash(in.Password)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "Password: "+err.Error()+".")
		return
	}
	u, err := s.Store.CreateLocalUser(in.Username, in.Email, in.Role, hash)
	if errors.Is(err, store.ErrNameTaken) {
		writeErr(w, http.StatusConflict, "That username is taken.")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Database error.")
		return
	}
	writeJSON(w, http.StatusCreated, u)
}

// userSetPassword (admin): give a local account a new password; it's signed out everywhere.
func (s *Server) userSetPassword(w http.ResponseWriter, r *http.Request, _ *store.User) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	u, err := s.Store.UserByID(id)
	if err != nil || !u.Local {
		writeErr(w, http.StatusNotFound, "That isn't a local account.")
		return
	}
	var in struct {
		Password string `json:"password"`
	}
	json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in)
	hash, err := password.Hash(in.Password)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "Password: "+err.Error()+".")
		return
	}
	if err := s.Store.SetPassword(id, hash); err != nil {
		writeErr(w, http.StatusInternalServerError, "Database error.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// myPassword: a local account changes its own password (current one needed). Other sessions end; this
// one gets a fresh session.
func (s *Server) myPassword(w http.ResponseWriter, r *http.Request, u *store.User) {
	if !u.Local {
		writeErr(w, http.StatusBadRequest, "Plex accounts sign in with Plex.")
		return
	}
	var in struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in)
	if !checkPassword(in.Current, s.Store.PasswordHash(u.ID)) {
		writeErr(w, http.StatusForbidden, "Your current password isn't right.")
		return
	}
	hash, err := password.Hash(in.New)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "New password: "+err.Error()+".")
		return
	}
	if err := s.Store.SetPassword(u.ID, hash); err != nil {
		writeErr(w, http.StatusInternalServerError, "Database error.")
		return
	}
	token, err := s.Store.CreateSession(u.ID)
	if err == nil {
		s.setSession(w, r, token)
	}
	w.WriteHeader(http.StatusNoContent)
}
