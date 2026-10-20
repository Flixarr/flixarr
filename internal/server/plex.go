package server

import (
	"encoding/json"
	"net/http"
	"strconv"

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

// plexWeb sets where Play on Plex and Open in Plex go: {url}, blank for app.plex.tv.
func (s *Server) plexWeb(w http.ResponseWriter, r *http.Request, _ *store.User) {
	var in struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "Bad request.")
		return
	}
	if _, err := s.Library.SetWebURL(in.URL); err != nil {
		writeErr(w, http.StatusBadRequest, capitalize(err.Error())+".")
		return
	}
	s.plexStatus(w, r, nil)
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

// plexAddresses lists the connected server's addresses, each tried now (#114).
func (s *Server) plexAddresses(w http.ResponseWriter, r *http.Request, _ *store.User) {
	list, auto, err := s.Library.Addresses(r.Context())
	if err != nil {
		writeErr(w, http.StatusBadGateway, capitalize(err.Error())+".")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"addresses": list, "auto": auto, "current": s.Store.Setting("plex_server_url", "")})
}

// plexAddress picks the address Flixarr uses: {"uri": ""} = Automatic (the fastest), or one to use.
func (s *Server) plexAddress(w http.ResponseWriter, r *http.Request, _ *store.User) {
	var in struct {
		URI string `json:"uri"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "Bad request.")
		return
	}
	st, err := s.Library.SetAddress(r.Context(), in.URI)
	if err != nil {
		writeErr(w, http.StatusBadGateway, capitalize(err.Error())+".")
		return
	}
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

// plexSeasons: episodes in Plex per season of a series, for the series page and the request window.
// plexEpisodes is which episodes of a series are in Plex, by season (the report window offers only those).
func (s *Server) plexEpisodes(w http.ResponseWriter, r *http.Request, _ *store.User) {
	id, _ := strconv.Atoi(r.PathValue("tmdbId"))
	got, err := s.Library.Episodes(r.Context(), id)
	if err != nil {
		s.Log.Warn("plex episodes", "err", err)
		got = map[int][]int{}
	}
	writeJSON(w, http.StatusOK, got)
}

func (s *Server) plexSeasons(w http.ResponseWriter, r *http.Request, _ *store.User) {
	id, _ := strconv.Atoi(r.PathValue("tmdbId"))
	got, err := s.Library.Seasons(r.Context(), id)
	if err != nil {
		s.Log.Warn("plex seasons", "err", err)
		got = map[int]int{} // the page works without it
	}
	writeJSON(w, http.StatusOK, got)
}
