package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/Flixarr/flixarr/internal/store"
)

// events streams job start/finish events to admins (Server-Sent Events), so the app can show
// exactly when a job is talking to Radarr or Sonarr. Some reverse proxies cut long-held
// connections (over HTTP/3 that shows as a QUIC protocol error in the browser), so the stream ends
// itself cleanly after a minute and tells the browser to reconnect half a second later.
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
	fmt.Fprint(w, "retry: 500\n: connected\n\n")
	fl.Flush()
	keep := time.NewTicker(20 * time.Second)
	defer keep.Stop()
	end := time.After(55 * time.Second)
	for {
		select {
		case <-r.Context().Done():
			return
		case <-end:
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
