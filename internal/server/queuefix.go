package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/Flixarr/flixarr/internal/requests"
	"github.com/Flixarr/flixarr/internal/store"
)

// queueFix fixes a download stuck in Radarr's or Sonarr's queue (#129): {"action": "different" | "retry" |
// "remove"}. For people who manage requests, and people with Fix and redownload.
func (s *Server) queueFix(w http.ResponseWriter, r *http.Request, u *store.User) {
	if !u.Can("manageRequests") && !u.Can("redownload") {
		writeErr(w, http.StatusForbidden, "Only people who manage requests can fix downloads.")
		return
	}
	kind := r.PathValue("kind")
	id, err := strconv.Atoi(r.PathValue("id"))
	if (kind != "radarr" && kind != "sonarr") || err != nil || id <= 0 {
		writeErr(w, http.StatusNotFound, "Not found.")
		return
	}
	var in struct {
		Action string `json:"action"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "Bad request.")
		return
	}
	f, err := s.Requests.FixDownload(r.Context(), kind, id, in.Action)
	if err != nil {
		writeErr(w, http.StatusBadGateway, capitalize(err.Error())+".")
		return
	}
	if f.TmdbID > 0 {
		s.Store.AddTitleEvent(f.Type, f.TmdbID, "queuefix", requests.QueueActions[in.Action], u.ID, "")
	}
	// The cards and the request catch up now rather than on the next pass.
	go func() {
		ctx, cancel := context.WithTimeout(s.BaseCtx, 30*time.Second)
		defer cancel()
		if v, ok, _ := s.Store.FirstServer(kind); ok {
			s.Arr.RefreshOne(ctx, v)
		}
		if f.ArrID > 0 {
			s.Requests.Nudge(ctx, f.Type, f.ArrID)
		}
	}()
	writeJSON(w, http.StatusOK, map[string]string{"done": requests.QueueActions[in.Action]})
}

// requestAttention counts the requests in Needs attention for this person (the red number on Requests):
// for people who manage requests, everyone's failed requests, requests waiting for approval and stuck
// downloads; for everyone else, their own failed requests and stuck downloads.
func (s *Server) requestAttention(w http.ResponseWriter, r *http.Request, u *store.User) {
	manage := u.Can("manageRequests")
	list, err := s.Store.RequestsIn("failed", "pending", "sent", "grabbed", "downloading", "importing")
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Database error.")
		return
	}
	stuck := s.stuckDownloads()
	n := 0
	for _, x := range list {
		if !manage && x.UserID != u.ID {
			continue
		}
		switch {
		case x.Status == "failed":
			n++
		case x.Status == "pending":
			if manage {
				n++
			}
		case x.ArrID > 0 && stuck[fmt.Sprintf("%s:%d", x.Type, x.ArrID)]:
			n++
		}
	}
	writeJSON(w, http.StatusOK, map[string]int{"count": n})
}

// stuckDownloads lists the titles ("movie:12", by their Radarr/Sonarr ID) with a download stuck right now.
func (s *Server) stuckDownloads() map[string]bool {
	out := map[string]bool{}
	servers, _ := s.Store.Servers()
	for _, v := range servers {
		snap, ok := s.Arr.Get(v.ID)
		if !ok {
			continue
		}
		for _, q := range snap.Queue {
			if (q.State == "warning" || q.State == "error") && q.ArrID > 0 {
				out[fmt.Sprintf("%s:%d", q.Type, q.ArrID)] = true
			}
		}
	}
	return out
}
