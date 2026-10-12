package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/Flixarr/flixarr/internal/arr"
	"github.com/Flixarr/flixarr/internal/profiles"
	"github.com/Flixarr/flixarr/internal/requests"
	"github.com/Flixarr/flixarr/internal/store"
)

// ---------- requests ----------

func (s *Server) requestList(w http.ResponseWriter, r *http.Request, u *store.User) {
	owner := u.ID
	if u.Can("viewRequests") || u.Can("manageRequests") {
		owner = 0 // everyone's
	}
	list, err := s.Store.Requests(owner, 200)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Database error.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"requests": list, "approvalRequired": s.Requests.ApprovalRequired()})
}

func (s *Server) requestCreate(w http.ResponseWriter, r *http.Request, u *store.User) {
	var in requests.New
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384)).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "Bad request.")
		return
	}
	req, err := s.Requests.Create(r.Context(), u, in)
	if errors.Is(err, requests.ErrNotAllowed) {
		writeErr(w, http.StatusForbidden, "You can't request "+map[string]string{"movie": "movies", "series": "series"}[in.Type]+".")
		return
	}
	if errors.Is(err, requests.ErrInPlex) {
		writeErr(w, http.StatusConflict, "That's already on Plex.")
		return
	}
	if errors.Is(err, requests.ErrAlready) {
		writeErr(w, http.StatusConflict, "That's already been requested.")
		return
	}
	if err != nil {
		writeErr(w, http.StatusBadRequest, capitalize(err.Error())+".")
		return
	}
	writeJSON(w, http.StatusCreated, req)
}

// requestStatus answers "is this requested?" for a batch of titles: ?keys=movie:603,series:1396.
func (s *Server) requestStatus(w http.ResponseWriter, r *http.Request, _ *store.User) {
	var keys []string
	for _, k := range strings.Split(r.URL.Query().Get("keys"), ",") {
		if k = strings.TrimSpace(k); k != "" && len(keys) < 200 {
			keys = append(keys, k)
		}
	}
	got, err := s.Store.RequestStatusFor(keys)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Database error.")
		return
	}
	type brief struct {
		ID       int64  `json:"id"`
		Status   string `json:"status"`
		Progress int    `json:"progress"`
		Plex     string `json:"plex,omitempty"` // opens it in Plex
	}
	out := map[string]brief{}
	for k, v := range got {
		out[k] = brief{ID: v.ID, Status: v.Status, Progress: v.Progress}
	}
	// In Plex: a movie is available whoever added it. A series gets the Plex link, but an open request
	// for more seasons keeps its own status.
	if inPlex, err := s.Library.Lookup(keys); err == nil {
		for k, it := range inPlex {
			b, requested := out[k]
			if strings.HasPrefix(k, "movie:") || !requested || b.Status == "available" || b.Status == "failed" || b.Status == "declined" {
				b.Status, b.Progress = "available", 100
			}
			b.Plex = it.URL
			out[k] = b
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) requestByID(w http.ResponseWriter, r *http.Request) (store.Request, bool) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	req, err := s.Store.Request(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "That request doesn't exist.")
		return req, false
	}
	return req, true
}

func (s *Server) requestApprove(w http.ResponseWriter, r *http.Request, u *store.User) {
	req, ok := s.requestByID(w, r)
	if !ok {
		return
	}
	if req.Status != "pending" && req.Status != "declined" {
		writeErr(w, http.StatusConflict, "Only waiting or declined requests can be approved.")
		return
	}
	s.Store.DecideRequest(req.ID, u.ID, "approved")
	req.Status = "approved"
	writeJSON(w, http.StatusOK, s.Requests.Send(r.Context(), req))
}

func (s *Server) requestDecline(w http.ResponseWriter, r *http.Request, u *store.User) {
	req, ok := s.requestByID(w, r)
	if !ok {
		return
	}
	if req.Status != "pending" {
		writeErr(w, http.StatusConflict, "Only waiting requests can be declined.")
		return
	}
	s.Store.DecideRequest(req.ID, u.ID, "declined")
	req, _ = s.Store.Request(req.ID)
	writeJSON(w, http.StatusOK, req)
}

func (s *Server) requestRetry(w http.ResponseWriter, r *http.Request, _ *store.User) {
	req, ok := s.requestByID(w, r)
	if !ok {
		return
	}
	if req.Status != "failed" {
		writeErr(w, http.StatusConflict, "Only failed requests can be tried again.")
		return
	}
	writeJSON(w, http.StatusOK, s.Requests.Send(r.Context(), req))
}

// requestDelete removes a request: your own while it's waiting, or any request for an admin.
// Nothing is removed from Radarr/Sonarr.
func (s *Server) requestDelete(w http.ResponseWriter, r *http.Request, u *store.User) {
	req, ok := s.requestByID(w, r)
	if !ok {
		return
	}
	if !u.Can("manageRequests") && (req.UserID != u.ID || req.Status != "pending") {
		writeErr(w, http.StatusForbidden, "You can only cancel your own requests while they're waiting.")
		return
	}
	if err := s.Store.DeleteRequest(req.ID); err != nil {
		writeErr(w, http.StatusInternalServerError, "Database error.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// requestSettings reads or changes whether requests need approval (Settings → General).
func (s *Server) requestSettings(w http.ResponseWriter, r *http.Request, _ *store.User) {
	if r.Method == http.MethodPut {
		var in struct {
			ApprovalRequired bool `json:"approvalRequired"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&in); err != nil {
			writeErr(w, http.StatusBadRequest, "Bad request.")
			return
		}
		s.Store.SetSetting(requests.KeyApproval, strconv.FormatBool(in.ApprovalRequired))
	}
	writeJSON(w, http.StatusOK, map[string]bool{"approvalRequired": s.Requests.ApprovalRequired()})
}

// requestOptions is what the request window needs: where it goes, the qualities to pick from
// (admins only; Flixarr profiles first) and whether it will wait for approval. ?type=movie|series
func (s *Server) requestOptions(w http.ResponseWriter, r *http.Request, u *store.User) {
	kind := "radarr"
	if r.URL.Query().Get("type") == "series" {
		kind = "sonarr"
	}
	out := map[string]any{"connected": false, "server": arr.Kinds[kind], "approval": s.Requests.ApprovalRequired() && !u.Can("autoApprove"),
		"allowed": u.Can(map[string]string{"radarr": "requestMovies", "sonarr": "requestTv"}[kind])}
	v, ok, err := s.Store.FirstServer(kind)
	if err != nil || !ok || !v.Enabled {
		writeJSON(w, http.StatusOK, out)
		return
	}
	out["connected"], out["server"], out["defaultProfileId"], out["defaultProfileName"] = true, v.Name, v.QualityProfileID, v.QualityProfileName
	type prof struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	list := []prof{}
	if u.Can("advanced") {
		var all []prof
		if err := (&arr.Client{Kind: v.Kind, URL: v.URL, APIKey: v.APIKey}).Get(r.Context(), "qualityprofile", &all); err == nil {
			for _, p := range all {
				if profiles.IsFlixarrName(p.Name) {
					list = append(list, p)
				}
			}
			for _, p := range all {
				if !profiles.IsFlixarrName(p.Name) {
					list = append(list, p)
				}
			}
		}
	}
	out["profiles"] = list
	writeJSON(w, http.StatusOK, out)
}
