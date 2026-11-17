package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/Flixarr/flixarr/internal/live"
	"github.com/Flixarr/flixarr/internal/requests"
	"github.com/Flixarr/flixarr/internal/store"
)

// Fix all on Settings → Services runs here, in the background: the window closes right away and the
// Downloads section shows how far it got (live "queuefix" events), even after the page is left. One run at a time.

type fixAllItem struct {
	Kind string `json:"kind"`
	ID   int    `json:"id"`
}

// FixAllProgress is what the page shows: how many of Total are done, which ones left the queue
// ("sonarr:123"), and how many couldn't be fixed.
type FixAllProgress struct {
	Running bool     `json:"running"`
	Total   int      `json:"total"`
	Done    int      `json:"done"`
	Failed  int      `json:"failed"`
	Fixed   []string `json:"fixed"`
}

var fixAll struct {
	sync.Mutex
	p FixAllProgress
}

func fixAllState() FixAllProgress {
	fixAll.Lock()
	defer fixAll.Unlock()
	p := fixAll.p
	p.Fixed = append([]string(nil), p.Fixed...)
	return p
}

func canFixDownloads(u *store.User) bool { return u.Can("manageRequests") || u.Can("redownload") }

// queueFixAllGet is the current (or last) Fix all run, for a page opened in the middle of one.
func (s *Server) queueFixAllGet(w http.ResponseWriter, r *http.Request, u *store.User) {
	if !canFixDownloads(u) {
		writeErr(w, http.StatusForbidden, "Only people who manage requests can fix downloads")
		return
	}
	writeJSON(w, http.StatusOK, fixAllState())
}

// queueFixAll starts getting a different copy of each listed download: {"items": [{"kind": "sonarr", "id": 123}]}.
func (s *Server) queueFixAll(w http.ResponseWriter, r *http.Request, u *store.User) {
	if !canFixDownloads(u) {
		writeErr(w, http.StatusForbidden, "Only people who manage requests can fix downloads")
		return
	}
	var in struct {
		Items []fixAllItem `json:"items"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256*1024)).Decode(&in); err != nil || len(in.Items) == 0 {
		writeErr(w, http.StatusBadRequest, "Invalid request")
		return
	}
	for _, it := range in.Items {
		if (it.Kind != "radarr" && it.Kind != "sonarr") || it.ID <= 0 {
			writeErr(w, http.StatusBadRequest, "Invalid request")
			return
		}
	}
	fixAll.Lock()
	if fixAll.p.Running {
		fixAll.Unlock()
		writeErr(w, http.StatusConflict, "Fix all is already running")
		return
	}
	fixAll.p = FixAllProgress{Running: true, Total: len(in.Items), Fixed: []string{}}
	fixAll.Unlock()
	s.publishFixAll()
	go s.runFixAll(in.Items, u.ID)
	writeJSON(w, http.StatusAccepted, fixAllState())
}

func (s *Server) runFixAll(items []fixAllItem, userID int64) {
	defer func() {
		fixAll.Lock()
		fixAll.p.Running = false
		fixAll.Unlock()
		s.publishFixAll()
	}()
	nudge := map[string]map[int]bool{}
	for _, it := range items {
		if s.BaseCtx.Err() != nil {
			return
		}
		ctx, cancel := context.WithTimeout(s.BaseCtx, time.Minute)
		f, err := s.Requests.FixDownload(ctx, it.Kind, it.ID, "different")
		cancel()
		fixAll.Lock()
		fixAll.p.Done++
		if err != nil {
			fixAll.p.Failed++
			s.Log.Warn("fix all", "kind", it.Kind, "id", it.ID, "err", err)
		} else {
			fixAll.p.Fixed = append(fixAll.p.Fixed, fmt.Sprintf("%s:%d", it.Kind, it.ID))
		}
		fixAll.Unlock()
		if err == nil && f.TmdbID > 0 {
			s.Store.AddTitleEvent(f.Type, f.TmdbID, "queuefix", requests.QueueActions["different"], userID, "")
			s.Store.AddFixSteps(f.Type, f.TmdbID, 0, requests.QueueActions["different"], userID)
		}
		if err == nil && f.ArrID > 0 {
			if nudge[f.Type] == nil {
				nudge[f.Type] = map[int]bool{}
			}
			nudge[f.Type][f.ArrID] = true
		}
		s.publishFixAll()
	}
	// The queues and the requests catch up now rather than on the next pass.
	ctx, cancel := context.WithTimeout(s.BaseCtx, 2*time.Minute)
	defer cancel()
	for _, kind := range []string{"radarr", "sonarr"} {
		if v, ok, _ := s.Store.FirstServer(kind); ok {
			s.Arr.RefreshOne(ctx, v)
		}
	}
	for typ, ids := range nudge {
		for id := range ids {
			s.Requests.Nudge(ctx, typ, id)
		}
	}
}

func (s *Server) publishFixAll() {
	s.Live.Publish(live.Event{Type: "queuefix", Data: fixAllState(), For: canFixDownloads})
}
