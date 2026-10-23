package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/Flixarr/flixarr/internal/live"
	"github.com/Flixarr/flixarr/internal/requests"
	"github.com/Flixarr/flixarr/internal/store"
)

// liveStream sends changes to the browser as they happen (Server-Sent Events): request status, bell
// alerts, titles added to Plex, and job runs for admins. Some reverse proxies cut long-held connections,
// so the stream ends itself cleanly after about a minute; the browser reconnects half a second later
// with Last-Event-ID and gets what it missed (or "resync" if that's no longer possible).
func (s *Server) liveStream(w http.ResponseWriter, r *http.Request, u *store.User) {
	fl, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "Streaming isn't supported.")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	ch, missed, resync, stop := s.Live.Subscribe(r.Header.Get("Last-Event-ID"))
	defer stop()
	jobs, stopJobs := s.Jobs.Subscribe()
	defer stopJobs()
	fmt.Fprint(w, "retry: 500\n: connected\n\n")
	send := func(e live.Event) {
		if e.For != nil && !e.For(u) {
			return
		}
		b, _ := json.Marshal(e.Data)
		if e.ID != "" {
			fmt.Fprintf(w, "id: %s\n", e.ID)
		}
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", e.Type, b)
	}
	if resync {
		send(live.Event{Type: "resync"})
	}
	for _, e := range missed {
		send(e)
	}
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
		case e := <-ch:
			send(e)
		case j := <-jobs:
			if u.Can("admin") {
				send(live.Event{Type: "job", Data: j})
			}
		}
		fl.Flush()
	}
}

// PublishRequest sends a request's new state to the people who can see it: the requester, and anyone
// else signed in (everyone can see all requests).
func (s *Server) PublishRequest(r store.Request) {
	key := fmt.Sprintf("%s:%d", r.Type, r.TmdbID)
	// The title as a whole (a series can have other requests still open), not just this request.
	var brief any
	if got, err := s.briefs([]string{key}); err == nil {
		if b, ok := got[key]; ok {
			brief = b
		}
	}
	s.Live.Publish(live.Event{Type: "request", Data: map[string]any{"key": key, "brief": brief, "request": requests.ForViewer(r, false)}}) // For nil: everyone signed in
}
