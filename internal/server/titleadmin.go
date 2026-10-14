package server

import (
	"net/http"
	"strconv"

	"github.com/Flixarr/flixarr/internal/store"
)

// requestDetail is a request's details window: its timeline and what Radarr / Sonarr say now. Your own
// requests, or anyone's if you can see everyone's; release names, folders and links only for people who
// manage requests.
func (s *Server) requestDetail(w http.ResponseWriter, r *http.Request, u *store.User) {
	req, ok := s.requestByID(w, r)
	if !ok {
		return
	}
	if req.UserID != u.ID && !u.Can("viewRequests") && !u.Can("manageRequests") {
		writeErr(w, http.StatusNotFound, "That request doesn't exist.")
		return
	}
	writeJSON(w, http.StatusOK, s.Requests.Detail(r.Context(), req, u.Can("manageRequests")))
}

func titleParams(r *http.Request) (string, int, bool) {
	typ := r.PathValue("type")
	id, _ := strconv.Atoi(r.PathValue("id"))
	return typ, id, (typ == "movie" || typ == "series") && id > 0
}

// titleAdmin is a title's admin panel: what Radarr / Sonarr say about it, every request for it, and its
// blocklist, Plex and issue state.
func (s *Server) titleAdmin(w http.ResponseWriter, r *http.Request, _ *store.User) {
	typ, id, ok := titleParams(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "Bad request.")
		return
	}
	reqs, err := s.Store.RequestsForTitle(typ, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Database error.")
		return
	}
	issues, _ := s.Store.OpenIssuesForTitle(typ, id)
	key := typ + ":" + strconv.Itoa(id)
	plex := ""
	if got, err := s.Library.Lookup([]string{key}); err == nil {
		plex = got[key].URL
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"live":     s.Requests.LiveFor(r.Context(), typ, id, 0, nil, true),
		"requests": reqs,
		"blocked":  s.Store.IsBlocked(typ, id),
		"plex":     plex,
		"issues":   issues,
	})
}

// titleFix is Fix on any title (requested through Flixarr or not).
func (s *Server) titleFix(w http.ResponseWriter, r *http.Request, _ *store.User) {
	typ, id, ok := titleParams(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "Bad request.")
		return
	}
	done, err := s.Requests.FixTitle(r.Context(), typ, id)
	if err != nil {
		writeErr(w, http.StatusConflict, capitalize(err.Error())+".")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"done": done})
}
