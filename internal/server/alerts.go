package server

import (
	"net/http"

	"github.com/Flixarr/flixarr/internal/store"
)

// ---------- alerts (the bell) ----------

func (s *Server) alerts(w http.ResponseWriter, r *http.Request, u *store.User) {
	s.Requests.SeenAlerts(u.ID) // a request alert already seen, with nothing still on its way, goes away
	list, err := s.Store.ActiveAlerts(u.ID, u.Can("admin"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Database error.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"alerts": list})
}

func (s *Server) alertsSeen(w http.ResponseWriter, r *http.Request, u *store.User) {
	if err := s.Store.MarkAlertsSeen(u.ID); err != nil {
		writeErr(w, http.StatusInternalServerError, "Database error.")
		return
	}
	s.alerts(w, r, u)
}
