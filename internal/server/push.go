package server

import (
	"encoding/json"
	"net/http"

	"github.com/Flixarr/flixarr/internal/store"
)

// pushKey is the VAPID public key the browser subscribes with.
func (s *Server) pushKey(w http.ResponseWriter, r *http.Request, u *store.User) {
	k, err := s.Notify.VAPID()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Couldn't make the push keys.")
		return
	}
	subs, _ := s.Store.PushSubs(u.ID)
	writeJSON(w, http.StatusOK, map[string]any{"key": k.Public, "devices": len(subs), "on": s.Notify.Load("push").On})
}

func (s *Server) pushSubscribe(w http.ResponseWriter, r *http.Request, u *store.User) {
	var in struct {
		Endpoint string `json:"endpoint"`
		Keys     struct {
			P256dh string `json:"p256dh"`
			Auth   string `json:"auth"`
		} `json:"keys"`
		Device string `json:"device"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&in); err != nil || in.Endpoint == "" || in.Keys.P256dh == "" || in.Keys.Auth == "" {
		writeErr(w, http.StatusBadRequest, "Bad subscription.")
		return
	}
	if r.Method == http.MethodDelete {
		s.Store.DeletePushSub(in.Endpoint, u.ID)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if len(in.Device) > 120 {
		in.Device = in.Device[:120]
	}
	if err := s.Store.SavePushSub(store.PushSub{Endpoint: in.Endpoint, UserID: u.ID, P256dh: in.Keys.P256dh, Auth: in.Keys.Auth, Device: in.Device}); err != nil {
		writeErr(w, http.StatusInternalServerError, "Database error.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
