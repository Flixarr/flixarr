package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"testing"

	"github.com/Flixarr/flixarr/internal/secret"
	"github.com/Flixarr/flixarr/internal/store"
	"github.com/Flixarr/flixarr/internal/version"
)

func newChecker(t *testing.T, api string) *Checker {
	t.Helper()
	dir := t.TempDir()
	box, _ := secret.Load(dir)
	st, err := store.Open(dir, box)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return &Checker{Store: st, API: api, HTTP: http.DefaultClient}
}

var uuidV4 = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// The check sends exactly the documented fields, keeps the answer and hands it to OnResult.
func TestCheck(t *testing.T) {
	var got url.Values
	var ua string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/update" {
			http.NotFound(w, r)
			return
		}
		got, ua = r.URL.Query(), r.Header.Get("User-Agent")
		w.Write([]byte(`{"updateAvailable":true,"latest":{"version":"0.2.0","build":"300","commit":"abc"},"profiles":{"version":"v9","revs":{"standard":"r9"}}}`))
	}))
	defer srv.Close()
	c := newChecker(t, srv.URL+"/")
	var hooked *Result
	c.OnResult = func(r *Result) { hooked = r }

	r, err := c.Check(context.Background())
	if err != nil || !r.UpdateAvailable || r.Latest.Build != "300" || r.Profiles.Revs["standard"] != "r9" {
		t.Fatalf("result %+v %v", r, err)
	}
	if hooked != r {
		t.Error("OnResult not called with the result")
	}
	want := []string{"install", "version", "build", "channel", "os", "arch", "db"}
	if len(got) != len(want) {
		t.Errorf("sent %d fields, want %d: %v", len(got), len(want), got)
	}
	for _, k := range want {
		if got.Get(k) == "" {
			t.Errorf("missing %s", k)
		}
	}
	if !uuidV4.MatchString(got.Get("install")) || got.Get("db") != "sqlite" || got.Get("version") != version.Full() {
		t.Errorf("payload %v", got)
	}
	if ua != "Flixarr/"+version.Full() {
		t.Errorf("user agent %q", ua)
	}
	// The install ID stays the same from one check to the next.
	first := got.Get("install")
	c.Check(context.Background())
	if got.Get("install") != first {
		t.Error("install ID changed")
	}
	if last := c.Last(); !last.Available || last.Latest.Version != "0.2.0" || last.CheckedAt == "" || !last.Enabled {
		t.Errorf("last %+v", last)
	}
}

// Turned off (by the switch or the environment), nothing is sent.
func TestCheckOff(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits++ }))
	defer srv.Close()
	c := newChecker(t, srv.URL)
	c.SetEnabled(false)
	if _, err := c.Check(context.Background()); err == nil || c.Enabled() {
		t.Error("check ran while off")
	}
	c.SetEnabled(true)
	c.ForceOff = true
	if _, err := c.Check(context.Background()); err == nil || c.Enabled() || !c.Locked() || !c.Last().Locked {
		t.Error("environment switch didn't win")
	}
	if hits != 0 {
		t.Fatalf("%d requests sent while off", hits)
	}
}

func TestCheckErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("build") == "" {
			w.WriteHeader(http.StatusBadRequest)
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	c := newChecker(t, srv.URL)
	if _, err := c.Check(context.Background()); err == nil {
		t.Fatal("503 accepted")
	}
	if c.Last().CheckedAt != "" {
		t.Error("a failed check was stored")
	}
}

// A release sends its version without a build number: with one, the API would read 0.2.0 as a build on
// its way to 0.2.0 and offer 0.2.0 as an update to itself.
func TestReleasePayload(t *testing.T) {
	v, pre, b := version.Version, version.Pre, version.Build
	t.Cleanup(func() { version.Version, version.Pre, version.Build = v, pre, b })
	c := newChecker(t, "http://unused")
	version.Version, version.Pre, version.Build = "0.2.0", "", "245"
	if p := c.Payload(); p["version"] != "0.2.0" || p["build"] != "" {
		t.Errorf("release payload %v", p)
	}
	version.Pre = "dev"
	if p := c.Payload(); p["version"] != "0.2.0-dev.245" || p["build"] != "245" {
		t.Errorf("build payload %v", p)
	}
}
