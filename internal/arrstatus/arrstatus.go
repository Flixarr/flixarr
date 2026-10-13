// Package arrstatus keeps the latest status of each Radarr and Sonarr connection (queue, health,
// disk, indexers, download clients) for the Services cards. A job refreshes it every minute.
package arrstatus

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/Flixarr/flixarr/internal/arr"
	"github.com/Flixarr/flixarr/internal/store"
)

// Monitor holds the latest snapshot per server.
type Monitor struct {
	Store     *store.Store
	Log       *slog.Logger
	OnRefresh func() // after a refresh: the Services cards reload

	mu    sync.RWMutex
	snaps map[int64]arr.Snapshot
}

// Get returns the latest snapshot for a server.
func (m *Monitor) Get(id int64) (arr.Snapshot, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.snaps[id]
	return s, ok
}

// Forget drops a removed server's snapshot.
func (m *Monitor) Forget(v store.Server) {
	m.mu.Lock()
	delete(m.snaps, v.ID)
	m.mu.Unlock()
	m.Store.ResolveAlert(alertKey(v))
}

// RefreshOne reads one server now.
func (m *Monitor) RefreshOne(ctx context.Context, v store.Server) arr.Snapshot {
	c := &arr.Client{Kind: v.Kind, URL: v.URL, APIKey: v.APIKey}
	snap := c.Read(ctx, v.RootFolder)
	m.mu.Lock()
	if m.snaps == nil {
		m.snaps = map[int64]arr.Snapshot{}
	}
	if snap.Connected {
		t := snap.UpdatedAt
		snap.LastConnected = &t
	} else if prev, ok := m.snaps[v.ID]; ok {
		snap.LastConnected = prev.LastConnected
	}
	m.snaps[v.ID] = snap
	m.mu.Unlock()
	m.syncAlert(v, snap)
	return snap
}

// Refresh is the "Radarr / Sonarr: refresh status" job (every minute). A server that was idle last time
// (nothing in its queue, no health problems) is only read every 5 minutes: grabs, imports and health
// changes come in through its webhook (RefreshAll) when that's set up.
func (m *Monitor) Refresh(ctx context.Context) (string, error) { return m.refresh(ctx, false) }

// RefreshAll reads every connection now (a webhook said something changed).
func (m *Monitor) RefreshAll(ctx context.Context) (string, error) { return m.refresh(ctx, true) }

func (m *Monitor) refresh(ctx context.Context, force bool) (string, error) {
	read := false // only tell the cards when something was actually read
	defer func() {
		if read && m.OnRefresh != nil {
			m.OnRefresh()
		}
	}()
	servers, err := m.Store.Servers()
	if err != nil {
		return "", err
	}
	if len(servers) == 0 {
		return "No servers connected", nil
	}
	var wg sync.WaitGroup
	ok := make([]bool, len(servers))
	for i, v := range servers {
		if prev, has := m.Get(v.ID); !force && has && prev.Connected && len(prev.Queue) == 0 && len(prev.Health) == 0 && time.Since(prev.UpdatedAt) < 5*time.Minute {
			ok[i] = true // idle: read again in a few minutes, or when a webhook arrives
			continue
		}
		read = true
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok[i] = m.RefreshOne(ctx, v).Connected
		}()
	}
	wg.Wait()
	up := 0
	for _, b := range ok {
		if b {
			up++
		}
	}
	if up == len(servers) {
		return fmt.Sprintf("%d of %d connected", up, len(servers)), nil
	}
	return fmt.Sprintf("%d of %d connected", up, len(servers)), fmt.Errorf("%d not reachable", len(servers)-up)
}

// alertKey is the bell alert for one server: all of its problems are grouped into this one alert.
func alertKey(v store.Server) string { return fmt.Sprintf("arr:%s:%d", v.Kind, v.ID) }

// syncAlert raises, updates or clears the server's single bell alert from its snapshot.
func (m *Monitor) syncAlert(v store.Server, snap arr.Snapshot) {
	key := alertKey(v)
	a := store.Alert{Key: key, Title: v.Name, Link: "/settings#services"}
	if !snap.Connected {
		a.Severity, a.Detail, a.Sig = "error", "Not reachable. "+snap.Error, "offline"
	} else {
		stuck, failed := 0, 0
		for _, q := range snap.Queue {
			switch q.State {
			case "warning":
				stuck++
			case "error":
				failed++
			}
		}
		var parts, kinds []string
		if failed > 0 {
			parts, kinds = append(parts, count(failed, "failed download", "failed downloads")), append(kinds, "failed")
		}
		if stuck > 0 {
			parts, kinds = append(parts, count(stuck, "download needs attention", "downloads need attention")), append(kinds, "stuck")
		}
		if n := len(snap.Health); n > 0 {
			parts, kinds = append(parts, count(n, "health issue", "health issues")), append(kinds, "health")
		}
		if len(parts) == 0 {
			if err := m.Store.ResolveAlert(key); err != nil {
				m.Log.Warn("alert", "err", err)
			}
			return
		}
		a.Severity = "warning"
		if failed > 0 {
			a.Severity = "error"
		}
		a.Detail = strings.Join(parts, " · ")
		a.Sig = strings.Join(kinds, ",") // a new kind of problem turns it unread; one clearing doesn't
	}
	if err := m.Store.RaiseAlert(a); err != nil {
		m.Log.Warn("alert", "err", err)
	}
}

func count(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}
