// Package arrstatus keeps the latest status of each Radarr and Sonarr connection (queue, health,
// disk, indexers, download clients) for the Services cards. A job refreshes it every minute.
package arrstatus

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/Flixarr/flixarr/internal/arr"
	"github.com/Flixarr/flixarr/internal/store"
)

// Monitor holds the latest snapshot per server.
type Monitor struct {
	Store *store.Store
	Log   *slog.Logger

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
func (m *Monitor) Forget(id int64) {
	m.mu.Lock()
	delete(m.snaps, id)
	m.mu.Unlock()
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
	return snap
}

// Refresh reads every connection; it's the "Radarr / Sonarr: refresh status" job.
func (m *Monitor) Refresh(ctx context.Context) (string, error) {
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
