package profiles

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/Flixarr/flixarr/internal/store"
)

// Builds every real profile against real servers without saving anything:
// FLIXARR_LIVE_DOC=profiles.json FLIXARR_LIVE_RADARR=url|key FLIXARR_LIVE_SONARR=url|key go test ./internal/profiles -run LiveBuild -v
func TestLiveBuild(t *testing.T) {
	raw, err := os.ReadFile(os.Getenv("FLIXARR_LIVE_DOC"))
	if err != nil {
		t.Skip("set FLIXARR_LIVE_DOC to a profiles.json")
	}
	var d Doc
	if err := json.Unmarshal(raw, &d); err != nil {
		t.Fatal(err)
	}
	for kind, env := range map[string]string{"radarr": "FLIXARR_LIVE_RADARR", "sonarr": "FLIXARR_LIVE_SONARR"} {
		p := strings.SplitN(os.Getenv(env), "|", 2)
		if len(p) != 2 {
			continue
		}
		m := &Manager{}
		s, err := m.load(context.Background(), store.Server{Kind: kind, URL: p[0], APIKey: p[1]})
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		for _, prof := range d.Profiles {
			ap := prof.For(kind)
			ids := map[string]int{}
			for i, f := range ap.Profile.FormatItems {
				ids[f.Name] = 9000 + i // pretend IDs: nothing is created
			}
			b, err := buildProfile(ArrName(prof), ap.Profile, s.schema, ids, s.formats, s.languages)
			if err != nil {
				t.Errorf("%s %s: %v", kind, prof.ID, err)
				continue
			}
			allowed := []string{}
			for i := len(b.Items) - 1; i >= 0; i-- {
				if it := b.Items[i]; it.Allowed {
					if it.Quality != nil {
						allowed = append(allowed, it.Quality.Name)
					} else {
						allowed = append(allowed, it.Name)
					}
				}
			}
			same := 0
			for _, f := range ap.Profile.FormatItems {
				for _, lf := range s.formats {
					if lf.Name == f.Name && specHash(lf.Specifications) == specHash(d.CustomFormats[kind][f.Name].Specifications) {
						same++
					}
				}
			}
			t.Logf("%s %-12s ok: %d qualities (allowed best first: %v), cutoff %d, %d formats (%d already on the server, identical)", kind, prof.ID, len(b.Items), allowed, b.Cutoff, len(ap.Profile.FormatItems), same)
		}
	}
}
