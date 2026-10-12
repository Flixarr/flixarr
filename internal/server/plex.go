package server

import (
	"encoding/json"
	"net/http"

	"github.com/Flixarr/flixarr/internal/store"
)

// ---------- Plex server and libraries (Settings → Plex) ----------

func (s *Server) plexStatus(w http.ResponseWriter, r *http.Request, _ *store.User) {
	st, err := s.Library.Status(r.Context())
	if err != nil {
		writeErr(w, http.StatusBadGateway, capitalize(err.Error())+".")
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) plexServers(w http.ResponseWriter, r *http.Request, _ *store.User) {
	list, err := s.Library.Servers(r.Context())
	if err != nil {
		writeErr(w, http.StatusBadGateway, capitalize(err.Error())+".")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) plexConnect(w http.ResponseWriter, r *http.Request, _ *store.User) {
	var in struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in); err != nil || in.ID == "" {
		writeErr(w, http.StatusBadRequest, "Bad request.")
		return
	}
	st, err := s.Library.Connect(r.Context(), in.ID)
	if err != nil {
		writeErr(w, http.StatusBadGateway, capitalize(err.Error())+".")
		return
	}
	s.Jobs.RunNow(s.BaseCtx, "plex-full")
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) plexDisconnect(w http.ResponseWriter, r *http.Request, _ *store.User) {
	if err := s.Library.Disconnect(); err != nil {
		writeErr(w, http.StatusInternalServerError, "Database error.")
		return
	}
	st, _ := s.Library.Status(r.Context())
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) plexLibraries(w http.ResponseWriter, r *http.Request, _ *store.User) {
	var in struct {
		Keys []string `json:"keys"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "Bad request.")
		return
	}
	if in.Keys == nil {
		in.Keys = []string{}
	}
	if err := s.Library.SetSections(in.Keys); err != nil {
		writeErr(w, http.StatusInternalServerError, "Database error.")
		return
	}
	s.Jobs.RunNow(s.BaseCtx, "plex-full") // pick up (or drop) the changed libraries
	st, _ := s.Library.Status(r.Context())
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) plexScan(w http.ResponseWriter, r *http.Request, _ *store.User) {
	s.Jobs.RunNow(s.BaseCtx, "plex-full")
	w.WriteHeader(http.StatusAccepted)
}
