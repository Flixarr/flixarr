package seerr

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Flixarr/flixarr/internal/meta"
	"github.com/Flixarr/flixarr/internal/secret"
	"github.com/Flixarr/flixarr/internal/store"
)

// A real Seerr, read only, imported into a new empty database in a temp folder (never a real one).
// Radarr and Sonarr are only read. Runs when FLIXARR_LIVE_SEERR_URL and FLIXARR_LIVE_SEERR_KEY are set.
func TestLiveSeerr(t *testing.T) {
	u, k := os.Getenv("FLIXARR_LIVE_SEERR_URL"), os.Getenv("FLIXARR_LIVE_SEERR_KEY")
	if u == "" || k == "" {
		t.Skip("no live Seerr")
	}
	dir := t.TempDir()
	box, _ := secret.Load(dir)
	st, err := store.Open(dir, box)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	c, err := NewClient(u, k)
	if err != nil {
		t.Fatal(err)
	}
	im := &Importer{Store: st, Meta: meta.New("https://api.flixarr.app", "Flixarr/0.1.0 (import test)", st)}
	start := time.Now()
	p, _, _, err := im.Plan(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("preview in %s: version %s, users %+v, requests %+v, issues %+v, notes %v", time.Since(start).Round(time.Millisecond), p.Version, p.Users, p.Requests, p.Issues, p.Notes)
	for _, s := range p.Servers {
		t.Logf("server %s %s: %s", s.Kind, s.Name, s.Action)
	}
	if os.Getenv("FLIXARR_LIVE_SEERR_IMPORT") == "" {
		return
	}
	start = time.Now()
	res, err := im.Run(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("import in %s: %+v", time.Since(start).Round(time.Second), res)
	again, err := im.Run(context.Background(), c)
	if err != nil || again.Users+again.Requests+again.Issues != 0 {
		t.Fatalf("second run added things: %+v %v", again, err)
	}
	list, _ := st.Requests(0, 5)
	for _, r := range list {
		t.Logf("request %s (%s) %s by %s, created %s", r.Title, r.Year, r.Status, r.UserName, r.CreatedAt)
	}
}
