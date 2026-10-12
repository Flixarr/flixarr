package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/Flixarr/flixarr/internal/store"
)

func (s *Server) blockList(w http.ResponseWriter, r *http.Request, _ *store.User) {
	list, err := s.Store.Blocklist()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Database error.")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) blockAdd(w http.ResponseWriter, r *http.Request, u *store.User) {
	var in store.Blocked
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in); err != nil || (in.Type != "movie" && in.Type != "series") || in.TmdbID <= 0 || in.Title == "" {
		writeErr(w, http.StatusBadRequest, "Bad request.")
		return
	}
	if err := s.Store.Block(in, u.ID); err != nil {
		writeErr(w, http.StatusInternalServerError, "Database error.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) blockRemove(w http.ResponseWriter, r *http.Request, _ *store.User) {
	id, _ := strconv.Atoi(r.PathValue("id"))
	if err := s.Store.Unblock(r.PathValue("type"), id); err != nil {
		writeErr(w, http.StatusInternalServerError, "Database error.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
