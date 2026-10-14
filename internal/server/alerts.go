package server

import (
	"fmt"
	"net/http"

	"github.com/Flixarr/flixarr/internal/store"
)

// ---------- alerts (the bell) ----------

func (s *Server) alerts(w http.ResponseWriter, r *http.Request, u *store.User) {
	// Personal alerts already seen on an earlier look go away (a request with nothing still on its way,
	// "your issue was fixed").
	s.Requests.SeenAlerts(u.ID)
	if list, err := s.Store.ActiveAlerts(u.ID, store.CanNone); err == nil {
		for _, a := range list {
			if a.Key == fmt.Sprintf("issues:user:%d", u.ID) && !a.Unread {
				s.Store.ResolveAlert(a.Key)
			}
		}
	}
	s.listAlerts(w, u)
}

func (s *Server) listAlerts(w http.ResponseWriter, u *store.User) {
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
	s.listAlerts(w, u) // not alerts(): that clears what's seen, and these were only just seen

}
