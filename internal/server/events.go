package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/Flixarr/flixarr/internal/store"
)

// events streams job start/finish events to admins (Server-Sent Events), so the app can show
// exactly when a job is talking to Radarr or Sonarr.
func (s *Server) events(w http.ResponseWriter, r *http.Request, _ *store.User) {
	fl, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "Streaming isn't supported.")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	ch, stop := s.Jobs.Subscribe()
	defer stop()
	fmt.Fprint(w, ": connected\n\n")
	fl.Flush()
	keep := time.NewTicker(25 * time.Second)
	defer keep.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-keep.C:
			fmt.Fprint(w, ": keep-alive\n\n")
			fl.Flush()
		case e := <-ch:
			b, _ := json.Marshal(e)
			fmt.Fprintf(w, "event: job\ndata: %s\n\n", b)
			fl.Flush()
		}
	}
}
