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
	admin := u.Can("manageRequests")
	for i := range list {
		list[i] = requests.ForViewer(list[i], admin)
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
	var limit requests.LimitError
	if errors.As(err, &limit) {
		writeErr(w, http.StatusConflict, capitalize(limit.Msg)+".")
		return
	}
	if errors.Is(err, requests.ErrNotAllowed) {
		writeErr(w, http.StatusForbidden, "You can't request "+map[string]string{"movie": "movies", "series": "series"}[in.Type]+".")
		return
	}
	if errors.Is(err, requests.ErrBlocked) {
		writeErr(w, http.StatusForbidden, "That title can't be requested.")
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
	writeJSON(w, http.StatusCreated, requests.ForViewer(req, u.Can("manageRequests")))
}

// requestStatus answers "is this requested?" for a batch of titles: ?keys=movie:603,series:1396.
func (s *Server) requestStatus(w http.ResponseWriter, r *http.Request, _ *store.User) {
	var keys []string
	for _, k := range strings.Split(r.URL.Query().Get("keys"), ",") {
		if k = strings.TrimSpace(k); k != "" && len(keys) < 200 {
			keys = append(keys, k)
		}
	}
	out, err := s.briefs(keys)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Database error.")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// brief is where a title stands, for buttons and poster dots.
type brief struct {
	ID       int64  `json:"id"`
	Status   string `json:"status"`
	Progress int    `json:"progress"`
	Plex     string `json:"plex,omitempty"` // opens it in Plex
	Blocked  bool   `json:"blocked,omitempty"`
	// Series: what open requests already ask for, so the page can offer the rest.
	Asked *store.Coverage `json:"asked,omitempty"`
}

// briefs works out where each title stands ("movie:603" keys); titles with nothing to say are left out.
func (s *Server) briefs(keys []string) (map[string]brief, error) {
	got, err := s.Store.RequestStatusFor(keys)
	if err != nil {
		return nil, err
	}
	out := map[string]brief{}
	for k, v := range got {
		b := brief{ID: v.ID, Status: v.Status, Progress: v.Progress}
		if v.Type == "series" {
			if c, err := s.Store.SeriesCoverage(v.TmdbID); err == nil {
				b.Asked = &c
			}
		}
		out[k] = b
	}
	for _, k := range keys {
		typ, id, _ := strings.Cut(k, ":")
		n, _ := strconv.Atoi(id)
		if _, has := out[k]; !has && s.Store.IsBlocked(typ, n) {
			out[k] = brief{Status: "blocked", Blocked: true}
		}
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
	return out, nil
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
	req = s.Requests.Send(r.Context(), req)
	if req.Status != "failed" {
		s.Notify.Send(s.Notify.FromRequest("request_approved", req))
	}
	s.Requests.Alert(req)
	writeJSON(w, http.StatusOK, req)
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
	var in struct {
		Reason string `json:"reason"`
	}
	json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in) // the reason is optional
	s.Store.DecideRequest(req.ID, u.ID, "declined")
	req, _ = s.Store.Request(req.ID)
	if reason := strings.TrimSpace(in.Reason); reason != "" {
		if len(reason) > 500 {
			reason = reason[:500]
		}
		req.Error = reason // shown on the card and in the notification
		s.Store.UpdateRequest(req)
	}
	s.Notify.Send(s.Notify.FromRequest("request_declined", req))
	s.Requests.Publish(req)
	s.Requests.Alert(req)
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

// requestFix repairs a stuck request (missing from Radarr/Sonarr, not monitored, never searched) and says
// what it did.
func (s *Server) requestFix(w http.ResponseWriter, r *http.Request, _ *store.User) {
	req, ok := s.requestByID(w, r)
	if !ok {
		return
	}
	fixed, done, err := s.Requests.Fix(r.Context(), req)
	if err != nil {
		writeErr(w, http.StatusConflict, capitalize(err.Error())+".")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"request": fixed, "done": done})
}

// redownload replaces a bad file (needs the Redownload permission): {type, tmdbId, seasons}.
func (s *Server) redownload(w http.ResponseWriter, r *http.Request, u *store.User) {
	var in struct {
		Type    string `json:"type"`
		TmdbID  int    `json:"tmdbId"`
		Seasons []int  `json:"seasons"`
		Title   string `json:"title"`
		Year    string `json:"year"`
		Poster  string `json:"poster"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in); err != nil || (in.Type != "movie" && in.Type != "series") || in.TmdbID <= 0 {
		writeErr(w, http.StatusBadRequest, "Bad request.")
		return
	}
	done, err := s.Requests.Redownload(r.Context(), in.Type, in.TmdbID, in.Seasons, requests.For{UserID: u.ID, Title: in.Title, Year: in.Year, Poster: in.Poster})
	if err != nil {
		writeErr(w, http.StatusConflict, capitalize(err.Error())+".")
		return
	}
	s.Log.Info("redownload asked", "by", u.Username, "type", in.Type, "tmdb", in.TmdbID)
	writeJSON(w, http.StatusOK, map[string]any{"done": done})
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
	req.Status = "deleted"
	s.Requests.Publish(req)
	s.Requests.Forgotten()
	w.WriteHeader(http.StatusNoContent)
}

// requestSettings reads or changes Settings → Requests: whether requests need approval, and how series
// can be requested.
func (s *Server) requestSettings(w http.ResponseWriter, r *http.Request, _ *store.User) {
	if r.Method == http.MethodPut {
		var in struct {
			ApprovalRequired bool              `json:"approvalRequired"`
			TVModes          *requests.TVModes `json:"tvModes"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&in); err != nil {
			writeErr(w, http.StatusBadRequest, "Bad request.")
			return
		}
		s.Store.SetSetting(requests.KeyApproval, strconv.FormatBool(in.ApprovalRequired))
		if in.TVModes != nil {
			if _, err := s.Requests.SetModes(*in.TVModes); err != nil {
				writeErr(w, http.StatusBadRequest, capitalize(err.Error())+".")
				return
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"approvalRequired": s.Requests.ApprovalRequired(), "tvModes": s.Requests.Modes(), "episodesReady": requests.EpisodesReady})
}

// requestOptions is what the request window needs: where it goes, the qualities to pick from
// (admins only; Flixarr profiles first) and whether it will wait for approval. ?type=movie|series
func (s *Server) requestOptions(w http.ResponseWriter, r *http.Request, u *store.User) {
	kind := "radarr"
	if r.URL.Query().Get("type") == "series" {
		kind = "sonarr"
	}
	out := map[string]any{"connected": false, "server": arr.Kinds[kind], "approval": s.Requests.ApprovalRequired() && !u.Can("autoApprove"),
		"allowed": u.Can(map[string]string{"radarr": "requestMovies", "sonarr": "requestTv"}[kind]),
		"usage":   s.Requests.UsageFor(u, map[string]string{"radarr": "movie", "sonarr": "series"}[kind]),
		"tvModes": s.Requests.Modes()}
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
