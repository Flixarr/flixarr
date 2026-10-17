package server

import (
	"encoding/json"
	"net/http"

	"github.com/Flixarr/flixarr/internal/seerr"
	"github.com/Flixarr/flixarr/internal/store"
)

// Settings → Import (and the first-time setup step): bring users, requests, issues and Radarr/Sonarr
// over from Seerr, Overseerr or Jellyseerr. Preview reads only; Import runs in the background.

func (s *Server) seerrClient(w http.ResponseWriter, r *http.Request) (*seerr.Client, bool) {
	var in struct {
		URL string `json:"url"`
		Key string `json:"key"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "Bad request.")
		return nil, false
	}
	c, err := seerr.NewClient(in.URL, in.Key)
	if err != nil {
		writeErr(w, http.StatusBadRequest, capitalize(err.Error())+".")
		return nil, false
	}
	return c, true
}

func (s *Server) seerrState(w http.ResponseWriter, r *http.Request, _ *store.User) {
	writeJSON(w, http.StatusOK, s.Seerr.State())
}

func (s *Server) seerrPreview(w http.ResponseWriter, r *http.Request, _ *store.User) {
	c, ok := s.seerrClient(w, r)
	if !ok {
		return
	}
	p, _, _, err := s.Seerr.Plan(r.Context(), c)
	if err != nil {
		writeErr(w, http.StatusBadGateway, capitalize(err.Error())+".")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) seerrImport(w http.ResponseWriter, r *http.Request, _ *store.User) {
	c, ok := s.seerrClient(w, r)
	if !ok {
		return
	}
	if err := s.Seerr.Start(c); err != nil {
		writeErr(w, http.StatusConflict, capitalize(err.Error())+".")
		return
	}
	writeJSON(w, http.StatusAccepted, s.Seerr.State())
}
