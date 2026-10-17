package server

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/Flixarr/flixarr/internal/live"
	"github.com/Flixarr/flixarr/internal/store"
)

// ---------- alerts (the bell, #90) ----------

func (s *Server) alerts(w http.ResponseWriter, r *http.Request, u *store.User) {
	list, err := s.Store.Bell(u.ID, u.Can)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Database error.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"alerts": list})
}

// alertsOpened: the bell was opened; nothing in it is new any more (still untapped).
func (s *Server) alertsOpened(w http.ResponseWriter, r *http.Request, u *store.User) {
	s.Store.OpenedBell(u.ID)
	s.tellBell(u.ID) // their other devices drop the red too
	s.alerts(w, r, u)
}

// alertTap: one alert was tapped; it comes off the number.
func (s *Server) alertTap(w http.ResponseWriter, r *http.Request, u *store.User) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	s.Store.TapAlert(u.ID, id)
	s.tellBell(u.ID)
	s.alerts(w, r, u)
}

// alertsReadAll taps everything in the bell (to-dos stay until they're handled).
func (s *Server) alertsReadAll(w http.ResponseWriter, r *http.Request, u *store.User) {
	s.Store.TapAll(u.ID, u.Can)
	s.tellBell(u.ID)
	s.alerts(w, r, u)
}

// tellBell redraws a person's bell on their other devices.
func (s *Server) tellBell(userID int64) {
	if s.Live != nil {
		s.Live.Publish(live.Event{Type: "alerts", For: live.ForAudience(fmt.Sprintf("user:%d", userID))})
	}
}
