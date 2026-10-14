package server

import (
	"fmt"
	"net/http"

	"github.com/Flixarr/flixarr/internal/store"
)

// ---------- alerts (the bell) ----------

func (s *Server) alerts(w http.ResponseWriter, r *http.Request, u *store.User) {
	s.Requests.SeenAlerts(u.ID) // a request alert already seen, with nothing still on its way, goes away
	if list, err := s.Store.ActiveAlerts(u.ID, store.CanNone); err == nil {
		for _, a := range list {
			if a.Key == fmt.Sprintf("issues:user:%d", u.ID) && !a.Unread {
				s.Store.ResolveAlert(a.Key) // "your issue was fixed", already seen
			}
		}
	}
	list, err := s.Store.ActiveAlerts(u.ID, u.Can)
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
