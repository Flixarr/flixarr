package arr

import "testing"

func TestTitleFits(t *testing.T) {
	for _, c := range []struct {
		title, app string
		want       bool
	}{
		{"Radarr", "Radarr", true},
		{"Movies", "Radarr", true},  // renamed instance
		{"Sonarr", "Radarr", false}, // the other one on a reused port
		{"Prowlarr", "Sonarr", false},
		{"TV (4K)", "Sonarr", true},
	} {
		if got := titleFits(c.title, c.app); got != c.want {
			t.Errorf("%q as %s: %v", c.title, c.app, got)
		}
	}
}
