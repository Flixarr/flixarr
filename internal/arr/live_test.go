package arr

import (
	"context"
	"os"
	"strings"
	"testing"
)

// Run against a real network: FLIXARR_LIVE_DISCOVER=1 go test ./internal/arr -run Live -v
func TestLiveDiscover(t *testing.T) {
	if os.Getenv("FLIXARR_LIVE_DISCOVER") == "" {
		t.Skip("set FLIXARR_LIVE_DISCOVER=1 to scan the real network")
	}
	for _, k := range []string{"radarr", "sonarr"} {
		t.Logf("%s: %v", k, Discover(context.Background(), k))
	}
}

// FLIXARR_LIVE_RADARR=url|key|folder and FLIXARR_LIVE_SONARR=... read real servers.
func TestLiveRead(t *testing.T) {
	for kind, env := range map[string]string{"radarr": "FLIXARR_LIVE_RADARR", "sonarr": "FLIXARR_LIVE_SONARR"} {
		v := os.Getenv(env)
		if v == "" {
			continue
		}
		p := strings.SplitN(v, "|", 3)
		s := (&Client{Kind: kind, URL: p[0], APIKey: p[1]}).Read(context.Background(), p[2])
		t.Logf("%s: connected=%v version=%s health=%v indexers=%d down=%d clients=%v clientsDown=%v folder=%s free=%d total=%d queue=%d err=%q",
			kind, s.Connected, s.Version, s.Health, s.Indexers, s.IndexersDown, s.Clients, s.ClientsDown, s.Folder, s.FreeSpace, s.TotalSpace, len(s.Queue), s.Error)
		for i, q := range s.Queue {
			if i < 4 {
				t.Logf("  %+v", q)
			}
		}
	}
}
