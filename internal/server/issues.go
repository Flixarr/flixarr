package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/Flixarr/flixarr/internal/store"
)

// issueKinds: what can be wrong, and the details for each (Other has none).
var issueKinds = map[string][]string{
	"Video":     {"Bad quality", "Wrong file", "Won't play"},
	"Audio":     {"Too quiet", "Out of sync", "Wrong language", "No audio"},
	"Subtitles": {"Missing", "Out of sync", "Wrong language"},
	"Other":     {},
}

// inLibrary: issues are about the actual file, so only titles in Plex (or with an available request).
func (s *Server) inLibrary(typ string, id int) bool {
	key := fmt.Sprintf("%s:%d", typ, id)
	if got, _ := s.Store.PlexItemsFor([]string{key}); len(got) > 0 {
		return true
	}
	got, _ := s.Store.RequestStatusFor([]string{key})
	return got[key].Status == "available"
}

func (s *Server) issueList(w http.ResponseWriter, r *http.Request, u *store.User) {
	owner := u.ID
	if u.Can("manageIssues") {
		owner = 0
	}
	status := r.URL.Query().Get("status")
	if status != "open" && status != "resolved" {
		status = ""
	}
	list, err := s.Store.Issues(owner, status)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Database error.")
		return
	}
	if u.Can("manageIssues") {
		s.syncIssueAlerts(nil, nil, "") // keeps the count right for issues reported before the bell counted them
	}
	writeJSON(w, http.StatusOK, map[string]any{"issues": list, "kinds": issueKinds})
}

func (s *Server) issueCreate(w http.ResponseWriter, r *http.Request, u *store.User) {
	var in store.Issue
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384)).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "Bad request.")
		return
	}
	subs, ok := issueKinds[in.Kind]
	switch {
	case !ok:
		writeErr(w, http.StatusBadRequest, "Pick what's wrong.")
		return
	case in.Sub != "" && !slices.Contains(subs, in.Sub):
		writeErr(w, http.StatusBadRequest, "Pick one of the listed problems.")
		return
	case (in.Type != "movie" && in.Type != "series") || in.TmdbID <= 0 || strings.TrimSpace(in.Title) == "":
		writeErr(w, http.StatusBadRequest, "Missing title.")
		return
	case !s.inLibrary(in.Type, in.TmdbID):
		writeErr(w, http.StatusConflict, "Issues can only be reported on titles in the library.")
		return
	}
	in.Note = strings.TrimSpace(in.Note)
	if len(in.Note) > 2000 {
		in.Note = in.Note[:2000]
	}
	if in.Kind == "Other" && in.Note == "" {
		writeErr(w, http.StatusBadRequest, "Say what's wrong in the details.")
		return
	}
	if in.Type == "movie" {
		in.Season, in.Episode, in.Parts = 0, 0, nil
	} else {
		if len(in.Parts) == 0 && in.Season > 0 {
			in.Parts = [][2]int{{in.Season, in.Episode}} // older clients sent one season/episode
		}
		if len(in.Parts) > 200 {
			in.Parts = in.Parts[:200]
		}
		in.Season, in.Episode = 0, 0
		if len(in.Parts) > 0 {
			in.Season, in.Episode = in.Parts[0][0], in.Parts[0][1]
		}
	}
	in.UserID = u.ID
	is, err := s.Store.CreateIssue(in)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Database error.")
		return
	}
	s.Notify.Send(s.Notify.FromIssue("issue_reported", is))
	s.syncIssueAlerts(&is, u, "reported")
	writeJSON(w, http.StatusCreated, is)
}

// issueByID returns the issue if the user may see it: their own, or anyone's with Manage issues.
func (s *Server) issueByID(w http.ResponseWriter, r *http.Request, u *store.User) (store.Issue, bool) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	is, err := s.Store.Issue(id)
	if errors.Is(err, store.ErrNotFound) || (err == nil && is.UserID != u.ID && !u.Can("manageIssues")) {
		writeErr(w, http.StatusNotFound, "That issue doesn't exist.")
		return is, false
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Database error.")
		return is, false
	}
	return is, true
}

func (s *Server) issueGet(w http.ResponseWriter, r *http.Request, u *store.User) {
	is, ok := s.issueByID(w, r, u)
	if !ok {
		return
	}
	comments, err := s.Store.Comments(is.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Database error.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"issue": is, "comments": comments})
}

func (s *Server) issueComment(w http.ResponseWriter, r *http.Request, u *store.User) {
	is, ok := s.issueByID(w, r, u)
	if !ok {
		return
	}
	var in struct {
		Body string `json:"body"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&in); err != nil || strings.TrimSpace(in.Body) == "" {
		writeErr(w, http.StatusBadRequest, "Write a comment first.")
		return
	}
	body := strings.TrimSpace(in.Body)
	if len(body) > 2000 {
		body = body[:2000]
	}
	cid, err := s.Store.AddComment(is.ID, u.ID, body)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Database error.")
		return
	}
	s.syncIssueAlerts(&is, u, fmt.Sprintf("reply c%d", cid))
	s.issueGet(w, r, u)
}

func (s *Server) issueStatus(status string) func(http.ResponseWriter, *http.Request, *store.User) {
	return func(w http.ResponseWriter, r *http.Request, u *store.User) {
		is, ok := s.issueByID(w, r, u)
		if !ok {
			return
		}
		if err := s.Store.SetIssueStatus(is.ID, u.ID, status); err != nil {
			writeErr(w, http.StatusInternalServerError, "Database error.")
			return
		}
		is, _ = s.Store.Issue(is.ID)
		if status == "resolved" {
			s.Notify.Send(s.Notify.FromIssue("issue_resolved", is))
		}
		s.syncIssueAlerts(&is, u, status)
		writeJSON(w, http.StatusOK, is)
	}
}

func (s *Server) issueDelete(w http.ResponseWriter, r *http.Request, u *store.User) {
	is, ok := s.issueByID(w, r, u)
	if !ok {
		return
	}
	if err := s.Store.DeleteIssue(is.ID); err != nil {
		writeErr(w, http.StatusInternalServerError, "Database error.")
		return
	}
	s.syncIssueAlerts(nil, u, "")
	w.WriteHeader(http.StatusNoContent)
}

// syncIssueAlerts keeps the bell current (grouped, Rule 19): one "N open issues" for people who manage
// issues (unread again on a new issue or a new reply), and one "Your issues" for a reporter when someone
// else replies or the issue is resolved. by is who made the change: it's already seen for them.
func (s *Server) syncIssueAlerts(changed *store.Issue, by *store.User, what string) {
	if n, sig := s.Store.OpenIssueSig(); n == 0 {
		s.Store.ResolveAlert("issues:open")
	} else {
		title := "1 open issue"
		if n > 1 {
			title = fmt.Sprintf("%d open issues", n)
		}
		detail := "Reported problems on titles in your library"
		if changed != nil && what == "reported" {
			detail = "New: " + changed.Kind + " on " + changed.Title
		} else if changed != nil && strings.HasPrefix(what, "reply") {
			detail = "New reply on " + changed.Title
		}
		s.Store.RaiseAlert(store.Alert{Key: "issues:open", Audience: "can:manageIssues", Severity: "warning", Title: title, Detail: detail, Link: "/issues", Sig: sig})
		if by != nil {
			s.Store.MarkAlertSeen(by.ID, "issues:open")
		}
	}
	if changed == nil || (by != nil && by.ID == changed.UserID) {
		return
	}
	key := fmt.Sprintf("issues:user:%d", changed.UserID)
	switch {
	case what == "resolved":
		s.Store.RaiseAlert(store.Alert{Key: key, Audience: fmt.Sprintf("user:%d", changed.UserID), Severity: "info",
			Title: "Your issues", Detail: changed.Title + " was fixed", Link: fmt.Sprintf("/issues?open=%d", changed.ID), Sig: fmt.Sprintf("f%d", changed.ID)})
	case strings.HasPrefix(what, "reply"):
		name := "Someone"
		if by != nil {
			name = by.Username
		}
		s.Store.RaiseAlert(store.Alert{Key: key, Audience: fmt.Sprintf("user:%d", changed.UserID), Severity: "info",
			Title: "Your issues", Detail: name + " replied on " + changed.Title, Link: fmt.Sprintf("/issues?open=%d", changed.ID), Sig: what})
	}
}
