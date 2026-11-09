package profiles

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Flixarr/flixarr/internal/secret"
	"github.com/Flixarr/flixarr/internal/store"
)

func ptr(f float64) *float64 { return &f }

// sizeArr keeps one quality definition and counts how often it's written.
type sizeArr struct {
	defs   []map[string]any
	writes int
}

func (a *sizeArr) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPut {
		a.writes++
		json.NewDecoder(r.Body).Decode(&a.defs)
	}
	json.NewEncoder(w).Encode(a.defs)
}

func sizeSetup(t *testing.T) (*Manager, store.Server, *sizeArr, *Doc) {
	t.Helper()
	a := &sizeArr{defs: []map[string]any{{"id": 1.0, "quality": map[string]any{"name": "Bluray-1080p"}, "minSize": 20.0, "preferredSize": 95.0, "maxSize": 100.0}}}
	srv := httptest.NewServer(a)
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	box, _ := secret.Load(dir)
	st, err := store.Open(dir, box)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	d := &Doc{Sizes: map[string]map[string]SizeLimit{"radarr": {"Bluray-1080p": {Min: 3, Preferred: ptr(1999), Max: ptr(2000)}}}}
	return &Manager{Store: st, Log: slog.Default()}, store.Server{ID: 7, Kind: "radarr", URL: srv.URL, APIKey: "k"}, a, d
}

func TestSizesApplyRestoreAndLeaveHandEditsAlone(t *testing.T) {
	m, v, a, d := sizeSetup(t)
	ctx := context.Background()
	if err := m.applySizes(ctx, v, d, false); err != nil {
		t.Fatal(err)
	}
	if a.defs[0]["minSize"] != 3.0 || a.defs[0]["maxSize"] != 2000.0 || m.SizeLimits(v.ID) != SizesApplied {
		t.Fatalf("not applied: %v, %s", a.defs[0], m.SizeLimits(v.ID))
	}
	// A second sync with nothing changed writes nothing.
	m.applySizes(ctx, v, d, false)
	if a.writes != 1 {
		t.Fatalf("wrote %d times, want 1", a.writes)
	}
	// Restore brings back what the server had, and later syncs leave it.
	if err := m.RestoreSizes(ctx, v); err != nil {
		t.Fatal(err)
	}
	if a.defs[0]["minSize"] != 20.0 {
		t.Fatalf("not restored: %v", a.defs[0])
	}
	m.applySizes(ctx, v, d, false)
	if a.defs[0]["minSize"] != 20.0 || m.SizeLimits(v.ID) != SizesRestored {
		t.Fatalf("a sync undid Restore: %v", a.defs[0])
	}
	// Use Flixarr's limits puts them back.
	if err := m.applySizes(ctx, v, d, true); err != nil {
		t.Fatal(err)
	}
	if a.defs[0]["minSize"] != 3.0 {
		t.Fatalf("not put back: %v", a.defs[0])
	}
	// Someone edits the page by hand: Flixarr notices and stops.
	a.defs[0]["minSize"] = 9.0
	m.applySizes(ctx, v, d, false)
	if a.defs[0]["minSize"] != 9.0 || m.SizeLimits(v.ID) != SizesEdited {
		t.Fatalf("a hand edit was overwritten: %v, %s", a.defs[0], m.SizeLimits(v.ID))
	}
}

func TestWithLanguageFillsInTheLibraryLanguage(t *testing.T) {
	f := CustomFormat{Name: "x", Specifications: []Spec{{Implementation: "LanguageSpecification", Fields: []Field{{Name: "value", Value: libraryToken}}}, {Implementation: "LanguageSpecification", Fields: []Field{{Name: "value", Value: -2}}}}}
	if !usesLanguage(f) {
		t.Fatal("should use the language")
	}
	got := withLanguage(f, 4)
	if got.Specifications[0].Fields[0].Value != 4 || got.Specifications[1].Fields[0].Value != -2 {
		t.Fatalf("got %+v", got.Specifications)
	}
	if f.Specifications[0].Fields[0].Value != libraryToken {
		t.Fatal("the shared document was changed")
	}
	if specHash(withLanguage(f, 4).Specifications) == specHash(withLanguage(f, 5).Specifications) {
		t.Fatal("a different language must count as different rules")
	}
}

// libArr is a small Radarr or Sonarr for the library move and the slow search.
type libArr struct {
	moved    map[string]any
	commands []map[string]any
	rows     string
}

func (a *libArr) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/api/v3/qualityprofile":
		w.Write([]byte(`[{"id":5,"name":"Flixarr · Standard"}]`))
	case r.Method == http.MethodPut:
		json.NewDecoder(r.Body).Decode(&a.moved)
		w.Write([]byte(`[]`))
	case r.URL.Path == "/api/v3/command":
		var c map[string]any
		json.NewDecoder(r.Body).Decode(&c)
		a.commands = append(a.commands, c)
		w.Write([]byte(`{}`))
	case r.URL.Path == "/api/v3/wanted/cutoff":
		w.Write([]byte(a.rows))
	default:
		w.Write([]byte(`[{"id":1},{"id":2},{"id":3}]`))
	}
}

func libSetup(t *testing.T, kind string, a *libArr) (*Manager, store.Server) {
	t.Helper()
	srv := httptest.NewServer(a)
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	box, _ := secret.Load(dir)
	st, err := store.Open(dir, box)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	v, err := st.SaveServer(store.Server{Kind: kind, Name: kind, URL: srv.URL, APIKey: "k", RootFolder: "/m", QualityProfileID: 1, QualityProfileName: "d", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	return &Manager{Store: st, Log: slog.Default()}, v
}

func TestMoveOnlySearchesNothing(t *testing.T) {
	a := &libArr{}
	m, v := libSetup(t, "radarr", a)
	n, err := m.MoveLibrary(context.Background(), v, 5, "")
	if err != nil || n != 3 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if a.moved["qualityProfileId"] != 5.0 || len(a.moved["movieIds"].([]any)) != 3 {
		t.Fatalf("moved %v", a.moved)
	}
	if len(a.commands) != 0 || m.LibrarySearching(v.ID) {
		t.Fatalf("Move only must not search: %v", a.commands)
	}
}

func TestMoveRefusesAProfileThatIsGone(t *testing.T) {
	a := &libArr{}
	m, v := libSetup(t, "radarr", a)
	if _, err := m.MoveLibrary(context.Background(), v, 99, ""); err == nil || a.moved != nil {
		t.Fatalf("err=%v moved=%v", err, a.moved)
	}
}

func TestSlowSearchTakesADailyBatchBySeasonThenStops(t *testing.T) {
	a := &libArr{rows: `{"totalRecords":4,"records":[{"id":10,"seriesId":1,"seasonNumber":1},{"id":11,"seriesId":1,"seasonNumber":1},{"id":12,"seriesId":1,"seasonNumber":2},{"id":13,"seriesId":2,"seasonNumber":1}]}`}
	m, v := libSetup(t, "sonarr", a)
	ctx := context.Background()
	if _, err := m.MoveLibrary(ctx, v, 5, "slow"); err != nil {
		t.Fatal(err)
	}
	if len(a.commands) != 0 {
		t.Fatalf("the move itself must not search: %v", a.commands)
	}
	m.SetPerDay(2)
	if _, err := m.RunLibrarySearch(ctx); err != nil {
		t.Fatal(err)
	}
	if len(a.commands) != 2 || a.commands[0]["name"] != "SeasonSearch" || a.commands[1]["seasonNumber"] != 2.0 {
		t.Fatalf("batch %v", a.commands)
	}
	a.rows = `{"totalRecords":0,"records":[]}`
	if _, err := m.RunLibrarySearch(ctx); err != nil {
		t.Fatal(err)
	}
	if m.LibrarySearching(v.ID) {
		t.Fatal("should stop once nothing is left")
	}
}
