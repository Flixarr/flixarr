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

	"github.com/Flixarr/flixarr/internal/arrstatus"
	"github.com/Flixarr/flixarr/internal/backup"
	"github.com/Flixarr/flixarr/internal/images"
	"github.com/Flixarr/flixarr/internal/jobs"
	"github.com/Flixarr/flixarr/internal/live"
	"github.com/Flixarr/flixarr/internal/maintainerr"
	"github.com/Flixarr/flixarr/internal/meta"
	"github.com/Flixarr/flixarr/internal/notify"
	"github.com/Flixarr/flixarr/internal/plex"
	"github.com/Flixarr/flixarr/internal/plexlib"
	"github.com/Flixarr/flixarr/internal/profiles"
	"github.com/Flixarr/flixarr/internal/requests"
	"github.com/Flixarr/flixarr/internal/seerr"
	"github.com/Flixarr/flixarr/internal/store"
	"github.com/Flixarr/flixarr/internal/update"
	"github.com/Flixarr/flixarr/internal/version"
)

const sessionCookie = "flixarr_session"

// Server holds everything the handlers need.
type Server struct {
	Store    *store.Store
	Plex     *plex.Client
	Meta     *meta.Client
	Images   *images.Cache
	Jobs     *jobs.Runner
	Update   *update.Checker
	Arr      *arrstatus.Monitor
	Profiles *profiles.Manager
	Requests *requests.Service
	Library  *plexlib.Library
	Notify   *notify.Notifier
	Maint    *maintainerr.Watcher
	Backups  *backup.Manager
	Seerr    *seerr.Importer
	Live     *live.Hub
	Web      fs.FS // built web app (index + assets)
	Log      *slog.Logger
	// BaseCtx outlives requests; jobs started with "Run now" use it.
	BaseCtx context.Context

	pinsMu sync.Mutex
	fails  failures            // wrong local passwords, for the sign-in pause
	pins   map[int64]time.Time // sign-in PINs this server created, so it only checks its own
}

// Handler returns the root HTTP handler.
func (s *Server) Handler() http.Handler {
	s.pins = map[int64]time.Time{}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/status", s.status)
	mux.HandleFunc("POST /api/webhook/{token}", s.webhook)
	mux.HandleFunc("POST /api/webhook/maintainerr/{token}", s.maintainerrHook)
	mux.HandleFunc("GET /api/maintainerr", s.auth(s.admin(s.maintainerrGet)))
	mux.HandleFunc("PUT /api/maintainerr", s.auth(s.admin(s.maintainerrSet)))
	mux.HandleFunc("DELETE /api/maintainerr", s.auth(s.admin(s.maintainerrRemove)))
	mux.HandleFunc("POST /api/maintainerr/find", s.auth(s.admin(s.maintainerrFind)))
	mux.HandleFunc("POST /api/maintainerr/test", s.auth(s.admin(s.maintainerrTest)))
	mux.HandleFunc("POST /api/maintainerr/webhook", s.auth(s.admin(s.maintainerrWebhook)))
	mux.HandleFunc("GET /api/leaving", s.auth(s.leaving))
	mux.HandleFunc("POST /api/leaving/{type}/{id}/{act}", s.auth(s.leavingAct))
	mux.HandleFunc("GET /api/webhook", s.auth(s.admin(s.webhookInfo)))
	mux.HandleFunc("GET /api/me", s.auth(s.me))
	mux.HandleFunc("GET /api/me/locale", s.auth(s.myLocale))
	mux.HandleFunc("PUT /api/me/locale", s.auth(s.myLocale))
	mux.HandleFunc("POST /api/setup/done", s.auth(s.admin(func(w http.ResponseWriter, r *http.Request, _ *store.User) {
		s.Store.SetSetting("setup_done", "true")
		w.WriteHeader(http.StatusNoContent)
	})))
	mux.HandleFunc("GET /api/me/stats", s.auth(s.myStats))
	mux.HandleFunc("GET /api/me/watchlist", s.auth(s.myWatchlist))
	mux.HandleFunc("GET /api/me/notifications", s.auth(s.myNotifications))
	mux.HandleFunc("PUT /api/me/notifications", s.auth(s.myNotifications))
	mux.HandleFunc("PUT /api/me/watchlist", s.auth(s.myWatchlist))
	mux.HandleFunc("POST /api/auth/plex/pin", s.plexPin)
	mux.HandleFunc("POST /api/auth/plex/check", s.plexCheck)
	mux.HandleFunc("POST /api/auth/logout", s.logout)
	mux.HandleFunc("POST /api/auth/local", s.localLogin)
	mux.HandleFunc("PUT /api/me/password", s.auth(s.myPassword))
	mux.HandleFunc("GET /api/meta/{path...}", s.auth(s.metaProxy))
	mux.HandleFunc("GET /api/update", s.auth(s.admin(s.updateInfo)))
	mux.HandleFunc("POST /api/update/check", s.auth(s.admin(s.updateCheck)))
	mux.HandleFunc("PUT /api/update/settings", s.auth(s.admin(s.updateSettings)))
	mux.HandleFunc("GET /api/image/{size}/{file}", s.auth(s.image))
	mux.HandleFunc("GET /api/avatar", s.auth(s.avatar))
	mux.HandleFunc("GET /api/cache", s.auth(s.admin(s.cacheInfo)))
	mux.HandleFunc("PUT /api/cache/settings", s.auth(s.admin(s.cacheSettings)))
	mux.HandleFunc("DELETE /api/cache/{kind}", s.auth(s.admin(s.cacheFlush)))
	mux.HandleFunc("GET /api/live", s.auth(s.liveStream))
	mux.HandleFunc("GET /api/events", s.auth(s.liveStream)) // older pages
	mux.HandleFunc("GET /api/requests", s.auth(s.requestList))
	mux.HandleFunc("POST /api/requests", s.auth(s.requestCreate))
	mux.HandleFunc("GET /api/requests/status", s.auth(s.requestStatus))
	mux.HandleFunc("GET /api/requests/options", s.auth(s.requestOptions))
	mux.HandleFunc("POST /api/requests/{id}/approve", s.auth(s.allow("manageRequests", s.requestApprove)))
	mux.HandleFunc("POST /api/requests/{id}/decline", s.auth(s.allow("manageRequests", s.requestDecline)))
	mux.HandleFunc("POST /api/requests/{id}/retry", s.auth(s.allow("manageRequests", s.requestRetry)))
	mux.HandleFunc("POST /api/requests/{id}/fix", s.auth(s.allow("manageRequests", s.requestFix)))
	mux.HandleFunc("GET /api/requests/{id}/detail", s.auth(s.requestDetail))
	mux.HandleFunc("GET /api/titles/{type}/{id}/admin", s.auth(s.allow("manageRequests", s.titleAdmin)))
	mux.HandleFunc("POST /api/titles/{type}/{id}/fix", s.auth(s.allow("manageRequests", s.titleFix)))
	mux.HandleFunc("POST /api/titles/{type}/{id}/delete", s.auth(s.allow("manageRequests", s.titleDelete)))
	mux.HandleFunc("GET /api/titles/{type}/{id}/episodes", s.auth(s.allow("redownload", s.titleEpisodes)))
	mux.HandleFunc("POST /api/titles/{type}/{id}/episodes/fix", s.auth(s.allow("redownload", s.titleFixEpisodes)))
	mux.HandleFunc("POST /api/redownload", s.auth(s.allow("redownload", s.redownload)))
	mux.HandleFunc("DELETE /api/requests/{id}", s.auth(s.requestDelete))
	mux.HandleFunc("GET /api/settings/requests", s.auth(s.admin(s.requestSettings)))
	mux.HandleFunc("PUT /api/settings/requests", s.auth(s.admin(s.requestSettings)))
	mux.HandleFunc("GET /api/library/{type}", s.auth(s.library))
	mux.HandleFunc("GET /api/discover/rows", s.auth(s.discoverRowsGet))
	mux.HandleFunc("PUT /api/discover/rows", s.auth(s.admin(s.discoverRowsSet)))
	mux.HandleFunc("GET /api/discover/recently-added", s.auth(s.recentlyAdded))
	mux.HandleFunc("GET /api/discover/recent-requests", s.auth(s.recentRequests))
	mux.HandleFunc("GET /api/issues", s.auth(s.issueList))
	mux.HandleFunc("POST /api/issues", s.auth(s.allow("reportIssues", s.issueCreate)))
	mux.HandleFunc("GET /api/issues/{id}", s.auth(s.issueGet))
	mux.HandleFunc("POST /api/issues/{id}/comments", s.auth(s.issueComment))
	mux.HandleFunc("POST /api/issues/{id}/resolve", s.auth(s.allow("manageIssues", s.issueStatus("resolved"))))
	mux.HandleFunc("POST /api/issues/{id}/reopen", s.auth(s.allow("manageIssues", s.issueStatus("open"))))
	mux.HandleFunc("DELETE /api/issues/{id}", s.auth(s.allow("manageIssues", s.issueDelete)))
	mux.HandleFunc("GET /api/alerts", s.auth(s.alerts))
	mux.HandleFunc("POST /api/alerts/opened", s.auth(s.alertsOpened))
	mux.HandleFunc("POST /api/alerts/read-all", s.auth(s.alertsReadAll))
	mux.HandleFunc("POST /api/alerts/{id}/tap", s.auth(s.alertTap))
	mux.HandleFunc("GET /api/services", s.auth(s.admin(s.services)))
	mux.HandleFunc("POST /api/services/{kind}/test", s.auth(s.admin(s.serviceTest)))
	mux.HandleFunc("POST /api/services/{kind}/discover", s.auth(s.admin(s.serviceDiscover)))
	mux.HandleFunc("GET /api/services/{kind}/webhook", s.auth(s.admin(s.webhookStatus)))
	mux.HandleFunc("POST /api/services/{kind}/webhook", s.auth(s.admin(s.webhookAdd)))
	mux.HandleFunc("POST /api/services/{kind}/refresh", s.auth(s.admin(s.serviceRefresh)))
	mux.HandleFunc("POST /api/services/{kind}/profiles", s.auth(s.admin(s.serviceProfilesAdd)))
	mux.HandleFunc("POST /api/services/{kind}/profiles/{key}/use-flixarr", s.auth(s.admin(s.serviceProfileUseFlixarr)))
	mux.HandleFunc("POST /api/services/{kind}/profiles/{key}/skip", s.auth(s.admin(s.serviceProfileSkip)))
	mux.HandleFunc("GET /api/services/{kind}/profiles/{key}/changes", s.auth(s.admin(s.serviceProfileChanges)))
	mux.HandleFunc("GET /api/profiles", s.auth(s.admin(s.profilesInfo)))
	mux.HandleFunc("PUT /api/services/{kind}", s.auth(s.admin(s.serviceSave)))
	mux.HandleFunc("DELETE /api/services/{kind}", s.auth(s.admin(s.serviceDelete)))
	mux.HandleFunc("GET /api/plex", s.auth(s.admin(s.plexStatus)))
	mux.HandleFunc("GET /api/plex/seasons/{tmdbId}", s.auth(s.plexSeasons))
	mux.HandleFunc("GET /api/plex/episodes/{tmdbId}", s.auth(s.plexEpisodes))
	mux.HandleFunc("GET /api/plex/servers", s.auth(s.admin(s.plexServers)))
	mux.HandleFunc("POST /api/plex/connect", s.auth(s.admin(s.plexConnect)))
	mux.HandleFunc("DELETE /api/plex", s.auth(s.admin(s.plexDisconnect)))
	mux.HandleFunc("PUT /api/plex/libraries", s.auth(s.admin(s.plexLibraries)))
	mux.HandleFunc("PUT /api/plex/web", s.auth(s.admin(s.plexWeb)))
	mux.HandleFunc("POST /api/plex/scan", s.auth(s.admin(s.plexScan)))
	mux.HandleFunc("GET /api/users", s.auth(s.admin(s.userList)))
	mux.HandleFunc("PUT /api/users/{id}", s.auth(s.admin(s.userUpdate)))
	mux.HandleFunc("DELETE /api/users/{id}", s.auth(s.admin(s.userDelete)))
	mux.HandleFunc("POST /api/users/local", s.auth(s.admin(s.userAddLocal)))
	mux.HandleFunc("PUT /api/users/{id}/password", s.auth(s.admin(s.userSetPassword)))
	mux.HandleFunc("GET /api/users/plex", s.auth(s.admin(s.userPlexList)))
	mux.HandleFunc("POST /api/users/plex", s.auth(s.admin(s.userPlexImport)))
	mux.HandleFunc("GET /api/settings/general", s.auth(s.admin(s.generalSettings)))
	mux.HandleFunc("PUT /api/settings/general", s.auth(s.admin(s.generalSettings)))
	mux.HandleFunc("GET /api/settings/users", s.auth(s.admin(s.userSettings)))
	mux.HandleFunc("PUT /api/settings/users", s.auth(s.admin(s.userSettings)))
	mux.HandleFunc("GET /api/push", s.auth(s.pushKey))
	mux.HandleFunc("POST /api/push", s.auth(s.pushSubscribe))
	mux.HandleFunc("DELETE /api/push", s.auth(s.pushSubscribe))
	mux.HandleFunc("GET /api/notifications", s.auth(s.admin(s.notifyList)))
	mux.HandleFunc("GET /api/import/seerr", s.auth(s.admin(s.seerrState)))
	mux.HandleFunc("POST /api/import/seerr/preview", s.auth(s.admin(s.seerrPreview)))
	mux.HandleFunc("POST /api/import/seerr", s.auth(s.admin(s.seerrImport)))
	mux.HandleFunc("POST /api/import/seerr/file/preview", s.auth(s.admin(s.seerrFilePreview)))
	mux.HandleFunc("POST /api/import/seerr/file", s.auth(s.admin(s.seerrFileImport)))
	mux.HandleFunc("POST /api/notifications", s.auth(s.admin(s.notifyAdd)))
	mux.HandleFunc("POST /api/notifications/test", s.auth(s.admin(s.notifyTest)))
	mux.HandleFunc("PUT /api/notifications/{id}", s.auth(s.admin(s.notifySave)))
	mux.HandleFunc("DELETE /api/notifications/{id}", s.auth(s.admin(s.notifyDelete)))
	mux.HandleFunc("GET /api/backups", s.auth(s.admin(s.backupList)))
	mux.HandleFunc("POST /api/backups", s.auth(s.admin(s.backupCreate)))
	mux.HandleFunc("PUT /api/backups/settings", s.auth(s.admin(s.backupSettings)))
	mux.HandleFunc("PUT /api/backups/offsite", s.auth(s.admin(s.backupOffsite)))
	mux.HandleFunc("GET /api/backups/{name}", s.auth(s.admin(s.backupDownload)))
	mux.HandleFunc("DELETE /api/backups/{name}", s.auth(s.admin(s.backupDelete)))
	mux.HandleFunc("POST /api/backups/{name}/restore", s.auth(s.admin(s.backupRestore)))
	mux.HandleFunc("GET /api/blocklist", s.auth(s.admin(s.blockList)))
	mux.HandleFunc("GET /api/settings/blocklist", s.auth(s.admin(s.blockSettings)))
	mux.HandleFunc("PUT /api/settings/blocklist", s.auth(s.admin(s.blockSettings)))
	mux.HandleFunc("POST /api/blocklist", s.auth(s.admin(s.blockAdd)))
	mux.HandleFunc("DELETE /api/blocklist/{type}/{id}", s.auth(s.admin(s.blockRemove)))
	mux.HandleFunc("GET /api/jobs", s.auth(s.admin(s.jobList)))
	mux.HandleFunc("PUT /api/jobs/{id}", s.auth(s.admin(s.jobSet)))
	mux.HandleFunc("POST /api/jobs/{id}/run", s.auth(s.admin(s.jobRun)))
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

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

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

// admin wraps an authenticated handler so only admins reach it.
func (s *Server) admin(h func(http.ResponseWriter, *http.Request, *store.User)) func(http.ResponseWriter, *http.Request, *store.User) {
	return func(w http.ResponseWriter, r *http.Request, u *store.User) {
		if !u.Can("admin") {
			writeErr(w, http.StatusForbidden, "Only admins can do that.")
			return
		}
		h(w, r, u)
	}
}

func secure(r *http.Request) bool {
	return r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
}

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
		// After the first sign-in: the rest of setup (Plex server, libraries, Radarr/Sonarr) until finished or skipped.
		"setupWizard": n > 0 && s.Store.Setting("setup_done", "") != "true",
		"version":     map[string]string{"version": version.Version, "build": version.Build, "full": version.Full(), "commit": version.Commit, "channel": version.Channel},
		"update":      s.Update.Last(),
		"imageCache":  s.Images.Enabled(),
		"signIn":      s.signIn(),
	})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request, u *store.User) {
	writeJSON(w, http.StatusOK, u)
}

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
			err = s.Store.SetSecretSetting("plex_admin_token", pin.AuthToken)
		}
		s.Log.Info("setup complete: admin created", "user", name)
	} else {
		u, err = s.Store.UserByPlexID(acct.ID)
		if errors.Is(err, store.ErrNotFound) {
			// Someone new: let them in if the Plex server is shared with them (and that's switched on).
			if s.signIn() == "local" {
				writeErr(w, http.StatusForbidden, "Sign in with the username and password the admin gave you.")
				return
			}
			if ok, aerr := s.Library.HasAccess(r.Context(), pin.AuthToken); s.plexSignInOpen() && aerr == nil && ok {
				u, err = s.Store.CreateUser(store.User{PlexID: acct.ID, Username: name, Email: acct.Email, Avatar: acct.Thumb, Role: s.defaultRole()})
				s.Log.Info("new user signed in", "user", name)
			} else {
				writeErr(w, http.StatusForbidden, "Your Plex account doesn't have access to this Flixarr yet. Ask the admin to add you.")
				return
			}
		} else if err == nil && s.signIn() == "local" && !u.Can("admin") && !u.Owner {
			writeErr(w, http.StatusForbidden, "Sign in with the username and password the admin gave you.")
			return
		} else if err == nil {
			err = s.Store.UpdatePlexProfile(u.ID, name, acct.Email, acct.Thumb)
			u.Username, u.Email, u.Avatar = name, acct.Email, acct.Thumb
		}
		if err == nil && u.Owner {
			err = s.Store.SetSecretSetting("plex_admin_token", pin.AuthToken) // the owner's token reads the Plex server list and who it's shared with
		}
	}
	if err == nil {
		err = s.Store.SetPlexToken(u.ID, pin.AuthToken) // for their Plex Watchlist requests
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

// updateInfo returns the update state plus exactly what a check sends (shown next to the toggle).
func (s *Server) updateInfo(w http.ResponseWriter, r *http.Request, _ *store.User) {
	writeJSON(w, http.StatusOK, map[string]any{"status": s.Update.Last(), "payload": s.Update.Payload()})
}

func (s *Server) updateCheck(w http.ResponseWriter, r *http.Request, _ *store.User) {
	if _, err := s.Update.Check(r.Context()); err != nil {
		writeErr(w, http.StatusBadGateway, "Couldn't check for updates: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.Update.Last())
}

func (s *Server) updateSettings(w http.ResponseWriter, r *http.Request, _ *store.User) {
	var in struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "Bad request.")
		return
	}
	if s.Update.Locked() && in.Enabled {
		writeErr(w, http.StatusConflict, "Update checks are turned off by the FLIXARR_UPDATE_CHECK setting on the server.")
		return
	}
	if err := s.Update.SetEnabled(in.Enabled); err != nil {
		writeErr(w, http.StatusInternalServerError, "Couldn't save.")
		return
	}
	writeJSON(w, http.StatusOK, s.Update.Last())
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		s.Store.DeleteSession(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: secure(r), SameSite: http.SameSiteLaxMode})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// metaProxy forwards /api/meta/<path> to the Flixarr API (through the cache) with this install's region and language.
func (s *Server) metaProxy(w http.ResponseWriter, r *http.Request, u *store.User) {
	q := url.Values{}
	for _, k := range []string{"q", "page", "genre", "sort"} {
		if v := r.URL.Query().Get(k); v != "" {
			q.Set(k, v)
		}
	}
	region, language := s.locale(r)
	q.Set("region", region)
	q.Set("language", language)
	body, status, err := s.Meta.Get(r.Context(), r.PathValue("path"), q)
	if err != nil {
		s.Log.Warn("Flixarr API", "err", err)
		writeErr(w, http.StatusBadGateway, "The Flixarr API didn't answer. Try again in a moment.")
		return
	}
	if status != http.StatusOK {
		writeErr(w, status, "Not found.")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	// no-store: the browser must not keep its own copy, or repeat visits never reach the cache (or its counters).
	// A cache hit here takes under a millisecond, so nothing is lost.
	w.Header().Set("Cache-Control", "no-store")
	w.Write(s.hideBlocked(body, u))
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
					switch p {
					case "manifest.webmanifest":
						w.Header().Set("Content-Type", "application/manifest+json")
					case "sw.js":
						w.Header().Set("Cache-Control", "no-cache") // a new service worker is picked up right away
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
		// no-transform: proxies and CDNs must not inject scripts (analytics beacons and the like) into the app.
		w.Header().Set("Cache-Control", "no-cache, no-transform")
		w.Write(shell)
	})
}
