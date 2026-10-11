// Package server is Flixarr's HTTP layer: the JSON API under /api and the embedded web app.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Flixarr/flixarr/internal/meta"
	"github.com/Flixarr/flixarr/internal/plex"
	"github.com/Flixarr/flixarr/internal/store"
	"github.com/Flixarr/flixarr/internal/version"
)

const sessionCookie = "flixarr_session"

// Server holds everything the handlers need.
type Server struct {
	Store *store.Store
	Plex  *plex.Client
	Meta  *meta.Client
	Web   fs.FS // built web app (index + assets)
	Log   *slog.Logger

	pinsMu sync.Mutex
	pins   map[int64]time.Time // sign-in PINs this server created, so it only checks its own
}

// Handler returns the root HTTP handler.
func (s *Server) Handler() http.Handler {
	s.pins = map[int64]time.Time{}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/status", s.status)
	mux.HandleFunc("GET /api/me", s.auth(s.me))
	mux.HandleFunc("POST /api/auth/plex/pin", s.plexPin)
	mux.HandleFunc("POST /api/auth/plex/check", s.plexCheck)
	mux.HandleFunc("POST /api/auth/logout", s.logout)
	mux.HandleFunc("GET /api/meta/{path...}", s.auth(s.metaProxy))
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { writeErr(w, http.StatusNotFound, "Unknown endpoint.") })
	mux.Handle("/", s.spa())
	return headers(mux)
}

// ---------- helpers ----------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) { writeJSON(w, status, map[string]string{"error": msg}) }

func headers(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

type ctxKey struct{}

// auth wraps a handler so it only runs for a signed-in user.
func (s *Server) auth(h func(http.ResponseWriter, *http.Request, *store.User)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookie)
		if err != nil || c.Value == "" {
			writeErr(w, http.StatusUnauthorized, "Sign in first.")
			return
		}
		u, err := s.Store.SessionUser(c.Value)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "Your session has ended. Sign in again.")
			return
		}
		h(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, u)), u)
	}
}

func secure(r *http.Request) bool { return r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" }

func (s *Server) setSession(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: token, Path: "/", HttpOnly: true, Secure: secure(r),
		SameSite: http.SameSiteLaxMode, MaxAge: int(store.SessionTTL.Seconds())})
}

// ---------- handlers ----------

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	n, err := s.Store.UserCount()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Database error.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"setupRequired": n == 0,
		"version": map[string]string{"version": version.Version, "build": version.Build, "full": version.Full(), "commit": version.Commit, "channel": version.Channel},
	})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request, u *store.User) { writeJSON(w, http.StatusOK, u) }

func (s *Server) plexPin(w http.ResponseWriter, r *http.Request) {
	pin, err := s.Plex.CreatePin(r.Context())
	if err != nil {
		s.Log.Warn("plex pin", "err", err)
		writeErr(w, http.StatusBadGateway, "Couldn't reach plex.tv. Try again in a moment.")
		return
	}
	s.pinsMu.Lock()
	for id, t := range s.pins { // forget PINs older than 15 minutes
		if time.Since(t) > 15*time.Minute {
			delete(s.pins, id)
		}
	}
	s.pins[pin.ID] = time.Now()
	s.pinsMu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"id": pin.ID, "code": pin.Code, "authUrl": s.Plex.AuthURL(pin.Code)})
}

func (s *Server) plexCheck(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&in); err != nil || in.ID == 0 {
		writeErr(w, http.StatusBadRequest, "Missing PIN id.")
		return
	}
	s.pinsMu.Lock()
	_, known := s.pins[in.ID]
	s.pinsMu.Unlock()
	if !known {
		writeErr(w, http.StatusBadRequest, "That sign-in expired. Start again.")
		return
	}
	pin, err := s.Plex.CheckPin(r.Context(), in.ID)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "Couldn't reach plex.tv.")
		return
	}
	if pin.AuthToken == "" {
		writeJSON(w, http.StatusOK, map[string]any{"done": false})
		return
	}
	s.pinsMu.Lock()
	delete(s.pins, in.ID)
	s.pinsMu.Unlock()

	acct, err := s.Plex.Me(r.Context(), pin.AuthToken)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "Couldn't read your Plex account.")
		return
	}
	name := acct.Username
	if name == "" {
		name = acct.Title
	}
	n, _ := s.Store.UserCount()
	var u *store.User
	if n == 0 {
		// First run: the first person to sign in owns this Flixarr and becomes the admin.
		u, err = s.Store.CreateUser(store.User{PlexID: acct.ID, Username: name, Email: acct.Email, Avatar: acct.Thumb, Role: "Admin"})
		if err == nil {
			err = s.Store.SetSetting("plex_admin_token", pin.AuthToken)
		}
		s.Log.Info("setup complete: admin created", "user", name)
	} else {
		u, err = s.Store.UserByPlexID(acct.ID)
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusForbidden, "Your Plex account doesn't have access to this Flixarr yet. Ask the admin to add you.")
			return
		}
		if err == nil {
			err = s.Store.UpdatePlexProfile(u.ID, name, acct.Email, acct.Thumb)
			u.Username, u.Email, u.Avatar = name, acct.Email, acct.Thumb
		}
	}
	if err != nil {
		s.Log.Error("sign-in", "err", err)
		writeErr(w, http.StatusInternalServerError, "Couldn't sign you in.")
		return
	}
	token, err := s.Store.CreateSession(u.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Couldn't start a session.")
		return
	}
	s.setSession(w, r, token)
	writeJSON(w, http.StatusOK, map[string]any{"done": true, "user": u})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		s.Store.DeleteSession(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: secure(r), SameSite: http.SameSiteLaxMode})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// metaProxy forwards /api/meta/<path> to the data server with this install's region and language.
func (s *Server) metaProxy(w http.ResponseWriter, r *http.Request, _ *store.User) {
	q := url.Values{}
	for _, k := range []string{"q", "page", "genre", "sort"} {
		if v := r.URL.Query().Get(k); v != "" {
			q.Set(k, v)
		}
	}
	q.Set("region", s.Store.Setting("region", "US"))
	q.Set("language", s.Store.Setting("language", "en-US"))
	body, status, err := s.Meta.Get(r.Context(), r.PathValue("path"), q)
	if err != nil {
		s.Log.Warn("data server", "err", err)
		writeErr(w, http.StatusBadGateway, "The Flixarr data server didn't answer. Try again in a moment.")
		return
	}
	if status != http.StatusOK {
		writeErr(w, status, "Not found.")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "private, max-age=300")
	w.Write(body)
}

// spa serves the built web app: real files when they exist, otherwise the app shell (200.html).
func (s *Server) spa() http.Handler {
	files := http.FileServer(http.FS(s.Web))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p != "" {
			if f, err := s.Web.Open(p); err == nil {
				st, _ := f.Stat()
				f.Close()
				if st != nil && !st.IsDir() {
					if strings.HasPrefix(p, "_app/immutable/") {
						w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
					}
					files.ServeHTTP(w, r)
					return
				}
			}
		}
		shell, err := fs.ReadFile(s.Web, "200.html")
		if err != nil {
			http.Error(w, "Web app not built.", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(shell)
	})
}
