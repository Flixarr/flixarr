package server

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/Flixarr/flixarr/internal/arr"
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
	open, _ := s.Store.OpenIssuesForTitle(typ, id)
	type adminIssue struct {
		store.Issue
		Last *store.Comment `json:"last,omitempty"` // the newest comment
	}
	issues := []adminIssue{}
	for _, is := range open {
		a := adminIssue{Issue: is}
		if is.Comments > 0 {
			if c, _ := s.Store.Comments(is.ID); len(c) > 0 {
				a.Last = &c[len(c)-1]
			}
		}
		issues = append(issues, a)
	}
	_, kept := s.Store.KeepFor(typ, id)
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
		"kept":     kept,
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

// arrName is Radarr for movies, Sonarr for series.
func arrName(typ string) string {
	if typ == "series" {
		return arr.Kinds["sonarr"]
	}
	return arr.Kinds["radarr"]
}

// titleRemove is Remove from Radarr / Sonarr: the title and its files go; requests for it end.
func (s *Server) titleRemove(w http.ResponseWriter, r *http.Request, u *store.User) {
	typ, id, ok := titleParams(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "Bad request.")
		return
	}
	in, err := s.Requests.RemoveFromArr(r.Context(), typ, id)
	if err != nil {
		writeErr(w, http.StatusConflict, capitalize(err.Error())+".")
		return
	}
	if !in {
		writeErr(w, http.StatusConflict, "It isn't in "+arrName(typ)+".")
		return
	}
	s.Log.Info("title removed", "type", typ, "tmdb", id, "by", u.Username)
	writeJSON(w, http.StatusOK, map[string]any{"done": "Removed it and its files from " + arrName(typ) + "."})
}

// titleClear is Clear data: Flixarr forgets the title (requests, issues, Plex and Maintainerr rows).
func (s *Server) titleClear(w http.ResponseWriter, r *http.Request, u *store.User) {
	typ, id, ok := titleParams(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "Bad request.")
		return
	}
	if err := s.Requests.ClearTitle(typ, id); err != nil {
		writeErr(w, http.StatusInternalServerError, "Database error.")
		return
	}
	s.syncIssueAlerts(nil, u, "")
	s.Log.Info("title data cleared", "type", typ, "tmdb", id, "by", u.Username)
	writeJSON(w, http.StatusOK, map[string]any{"done": "Cleared its data."})
}

// titleDelete is Delete everywhere: Maintainerr lets go of it, Radarr / Sonarr remove it with its files,
// Plex deletes what's left, and Flixarr clears its data. It stops at the first step that fails, so
// nothing is cleared while the files are still there.
func (s *Server) titleDelete(w http.ResponseWriter, r *http.Request, u *store.User) {
	typ, id, ok := titleParams(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "Bad request.")
		return
	}
	ctx := r.Context()
	done := []string{}
	if _, kept := s.Store.KeepFor(typ, id); kept && s.Maint != nil {
		if err := s.Maint.LetGo(ctx, typ, id); err != nil {
			writeErr(w, http.StatusConflict, "Maintainerr didn't let go of it: "+err.Error()+".")
			return
		}
		done = append(done, "Maintainerr no longer keeps it.")
	}
	if in, err := s.Requests.RemoveFromArr(ctx, typ, id); err != nil && !strings.Contains(err.Error(), "isn't connected") {
		writeErr(w, http.StatusConflict, capitalize(err.Error())+".")
		return
	} else if in {
		done = append(done, "Removed it and its files from "+arrName(typ)+".")
	}
	if gone, err := s.Library.Delete(ctx, typ, id); err != nil {
		writeErr(w, http.StatusConflict, strings.Join(append(done, capitalize(err.Error())+"."), " "))
		return
	} else if gone {
		done = append(done, "Deleted it from Plex.")
	}
	if err := s.Requests.ClearTitle(typ, id); err != nil {
		writeErr(w, http.StatusInternalServerError, "Database error.")
		return
	}
	s.syncIssueAlerts(nil, u, "")
	s.Log.Info("title deleted everywhere", "type", typ, "tmdb", id, "by", u.Username)
	writeJSON(w, http.StatusOK, map[string]any{"done": strings.Join(append(done, "Cleared its data."), " ")})
}
