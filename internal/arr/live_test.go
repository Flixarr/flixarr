package arr

import (
	"context"
	"os"
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
