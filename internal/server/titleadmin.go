package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/Flixarr/flixarr/internal/requests"
	"github.com/Flixarr/flixarr/internal/store"
)

// requestDetail is a request's details window: its timeline and what Radarr / Sonarr say now. Anyone
// signed in can open any request; release names, folders and links only for people who manage requests.
func (s *Server) requestDetail(w http.ResponseWriter, r *http.Request, u *store.User) {
	req, ok := s.requestByID(w, r)
	if !ok {
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
	history, _ := s.Store.TitleHistory(typ, id)
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
		"history":  history,
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

// titleDelete is the admin panel's Delete: {files, data}. files: Radarr / Sonarr delete the title with
// its files (Maintainerr lets go of it first if it kept it; Plex is asked to scan and drops it). data:
// Flixarr forgets its requests and issues. Files go first, so nothing is cleared if that fails.
func (s *Server) titleDelete(w http.ResponseWriter, r *http.Request, u *store.User) {
	typ, id, ok := titleParams(r)
	var in struct {
		Files bool `json:"files"`
		Data  bool `json:"data"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&in); err != nil || !ok || (!in.Files && !in.Data) {
		writeErr(w, http.StatusBadRequest, "Bad request.")
		return
	}
	if in.Files && !u.Can("redownload") {
		writeErr(w, http.StatusForbidden, "You don't have permission to delete files.")
		return
	}
	ctx := r.Context()
	done := []string{}
	if in.Files {
		if _, kept := s.Store.KeepFor(typ, id); kept && s.Maint != nil {
			if err := s.Maint.LetGo(ctx, typ, id); err != nil {
				writeErr(w, http.StatusConflict, "Maintainerr didn't let go of it: "+err.Error()+".")
				return
			}
		}
		found, err := s.Requests.RemoveFromArr(ctx, typ, id)
		if err != nil {
			writeErr(w, http.StatusConflict, capitalize(err.Error())+".")
			return
		}
		if !found {
			writeErr(w, http.StatusConflict, "It isn't in "+requests.ArrName(typ)+".")
			return
		}
		done = append(done, "Deleted it and its files from "+requests.ArrName(typ)+".")
		s.Store.AddTitleEvent(typ, id, "deleted", requests.ArrName(typ), u.ID, "")
	}
	if in.Data {
		if err := s.Requests.ClearTitle(typ, id); err != nil {
			writeErr(w, http.StatusInternalServerError, "Database error.")
			return
		}
		s.syncIssueAlerts(nil, u, "")
		done = append(done, "Cleared its requests and issues.")
		s.Store.AddTitleEvent(typ, id, "cleared", "", u.ID, "") // after the clear: history keeps this step
	}
	s.Log.Info("title deleted", "type", typ, "tmdb", id, "files", in.Files, "data", in.Data, "by", u.Username)
	writeJSON(w, http.StatusOK, map[string]any{"done": strings.Join(done, " ")})
}

// titleEpisodes is the Fix episodes window: a series' episodes in Sonarr and Plex.
func (s *Server) titleEpisodes(w http.ResponseWriter, r *http.Request, _ *store.User) {
	_, id, ok := titleParams(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "Bad request.")
		return
	}
	inPlex, _ := s.Library.Episodes(r.Context(), id)
	eps, err := s.Requests.Episodes(r.Context(), id, inPlex)
	if err != nil {
		writeErr(w, http.StatusConflict, capitalize(err.Error())+".")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"episodes": eps})
}

// titleFixEpisodes fixes picked episodes: {episodes: [[season, episode]], title, year, poster}.
func (s *Server) titleFixEpisodes(w http.ResponseWriter, r *http.Request, u *store.User) {
	_, id, ok := titleParams(r)
	var in struct {
		Episodes [][2]int `json:"episodes"`
		Title    string   `json:"title"`
		Year     string   `json:"year"`
		Poster   string   `json:"poster"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 65536)).Decode(&in); err != nil || !ok || len(in.Episodes) > 2000 {
		writeErr(w, http.StatusBadRequest, "Bad request.")
		return
	}
	done, err := s.Requests.FixEpisodes(r.Context(), id, in.Episodes, requests.For{UserID: u.ID, Title: in.Title, Year: in.Year, Poster: in.Poster})
	if err != nil {
		writeErr(w, http.StatusConflict, capitalize(err.Error())+".")
		return
	}
	s.Store.AddTitleEvent("series", id, "redownload", plexParts(in.Episodes), u.ID, "")
	writeJSON(w, http.StatusOK, map[string]any{"done": done})
}

// plexParts is "S1E3, S2E1" (the first few, then how many more).
func plexParts(eps [][2]int) string {
	out := []string{}
	for i, e := range eps {
		if i == 6 {
			out = append(out, fmt.Sprintf("and %d more", len(eps)-6))
			break
		}
		out = append(out, fmt.Sprintf("S%dE%d", e[0], e[1]))
	}
	return strings.Join(out, ", ")
}
