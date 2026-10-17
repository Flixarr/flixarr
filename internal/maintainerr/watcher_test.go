package maintainerr

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Flixarr/flixarr/internal/jobs"
	"github.com/Flixarr/flixarr/internal/secret"
	"github.com/Flixarr/flixarr/internal/store"
)

// fake is a Maintainerr with one movie collection: The Matrix (Plex 56) added 25 days ago (leaves in 3
// days, 28-day collection) and Heat (Plex 57) added today.
type fake struct {
	mu    sync.Mutex
	calls []string
}

func (f *fake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, _ := io.ReadAll(r.Body)
	f.calls = append(f.calls, r.Method+" "+r.URL.Path+" "+string(b))
	day := func(n int) string {
		return time.Now().Add(time.Duration(-n) * 24 * time.Hour).UTC().Format("2006-01-02 15:04:05.000")
	}
	switch {
	case r.URL.Path == "/api/app/status":
		w.Write([]byte(`{"version":"3.30.1"}`))
	case r.URL.Path == "/api/collections":
		w.Write([]byte(`[{"id":3,"title":"Movies leaving","type":"movie","isActive":true,"deleteAfterDays":28,"handledMediaAmount":5,"handledMediaSizeBytes":1000,"mediaCount":2},
			{"id":4,"title":"Old seasons","type":"season","isActive":true,"deleteAfterDays":10,"mediaCount":1}]`))
	case strings.HasPrefix(r.URL.Path, "/api/collections/media/3/content/"):
		fmt.Fprintf(w, `{"totalSize":2,"items":[{"mediaServerId":"56","tmdbId":603,"addDate":"%s"},{"mediaServerId":"57","tmdbId":949,"addDate":"%s"}]}`, day(25), day(0))
	case strings.HasPrefix(r.URL.Path, "/api/collections/media/4/content/"):
		fmt.Fprintf(w, `{"totalSize":1,"items":[{"mediaServerId":"90","tmdbId":1396,"addDate":"%s"}]}`, day(1))
	case r.URL.Path == "/api/settings":
		w.Write([]byte(`{"collection_handler_job_cron":"0 4 * * *"}`)) // daily at 4
	case r.URL.Path == "/api/rules/exclusion":
		w.Write([]byte(`{"code":1,"result":"Success"}`))
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (f *fake) did(prefix string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []string{}
	for _, c := range f.calls {
		if strings.HasPrefix(c, prefix) {
			out = append(out, c)
		}
	}
	return out
}

func TestSync(t *testing.T) {
	dir := t.TempDir()
	box, _ := secret.Load(dir)
	st, _ := store.Open(dir, box)
	t.Cleanup(func() { st.Close() })
	f := &fake{}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	st.SetSetting(KeyURL, srv.URL)
	u, _ := st.CreateUser(store.User{PlexID: 1, Username: "eryn", Role: "User"})
	st.CreateRequest(store.Request{Type: "movie", TmdbID: 603, Title: "The Matrix", UserID: u.ID, Status: "available", ArrID: 1})
	told := []string{}
	var changed []string
	w := &Watcher{Store: st, Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Leaving: func(r store.Request, at time.Time) { told = append(told, r.Title) },
		Changed: func(k []string) { changed = k }}

	// Keeping new requests switched off: nothing kept, and Eryn hears The Matrix leaves within a week.
	st.SetSetting(KeyProtect, "0")
	if _, err := w.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	now, _ := st.LeavingNow(w.Seen())
	if len(now) != 2 || now[0].TmdbID != 603 || now[0].Collection != "Movies leaving" {
		t.Fatalf("leaving: %+v", now) // season collections aren't shown as the show leaving
	}
	if at, _ := time.Parse(time.RFC3339, now[0].LeaveAt); time.Until(at) < 2*24*time.Hour || time.Until(at) > 4*24*time.Hour || at.In(time.Local).Hour() != 4 || at.Minute() != 0 {
		t.Fatalf("leave date: %s (the 4 AM run after its 28 days)", now[0].LeaveAt)
	}
	if len(changed) != 2 || len(f.did("POST /api/rules/exclusion")) != 0 {
		t.Fatalf("changed %v, exclusions %v", changed, f.did("POST /api/rules/exclusion"))
	}
	if len(told) != 1 || told[0] != "The Matrix" {
		t.Fatalf("told: %v", told)
	}
	w.Sync(context.Background())
	if len(told) != 1 {
		t.Fatalf("told again: %v", told)
	}
	// Switched on (30 days): Heat, requested and here just now, is kept out of the rules (as is The Matrix,
	// whose request also arrived within the month).
	st.SetSetting(KeyProtect, "30")
	st.CreateRequest(store.Request{Type: "movie", TmdbID: 949, Title: "Heat", UserID: u.ID, Status: "available", ArrID: 2})
	w.Sync(context.Background())
	if ex := f.did("POST /api/rules/exclusion"); len(ex) != 2 || !strings.Contains(strings.Join(ex, " "), `"mediaId":"57"`) {
		t.Fatalf("exclusions: %v", ex)
	}
	if k, ok := st.KeepFor("movie", 949); !ok || k.Reason != "fresh" {
		t.Fatalf("fresh keep: %+v %v", k, ok)
	}
	st.DropKeep("movie", 603)
	// Asking, then keeping: the ask clears and the keep lasts.
	if err := w.Ask("movie", 603, u.ID); err != nil {
		t.Fatal(err)
	}
	if asks, _ := st.KeepAsks(); len(asks) != 1 {
		t.Fatalf("asks: %v", asks)
	}
	if err := w.Keep(context.Background(), "movie", 603, u.ID); err != nil {
		t.Fatal(err)
	}
	if k, _ := st.KeepFor("movie", 603); k.Reason != "kept" {
		t.Fatalf("kept: %+v", k)
	}
	if asks, _ := st.KeepAsks(); len(asks) != 0 {
		t.Fatalf("asks after keep: %v", asks)
	}
	// A fresh keep that ran out: the rules see it again.
	st.DropKeep("movie", 949)
	st.SaveKeep(store.Keep{Type: "movie", TmdbID: 949, PlexID: "57", Reason: "fresh", UntilAt: time.Now().Add(-time.Hour).Format(time.RFC3339)})
	w.endFreshKeeps(context.Background(), &Client{URL: srv.URL})
	if _, ok := st.KeepFor("movie", 949); ok {
		t.Fatal("ran-out keep still there")
	}
	var last map[string]any
	ex := f.did("POST /api/rules/exclusion")
	json.Unmarshal([]byte(strings.TrimPrefix(ex[len(ex)-1], "POST /api/rules/exclusion ")), &last)
	if last["action"] != float64(1) {
		t.Fatalf("not unexcluded: %v", ex[len(ex)-1])
	}
}

// A title whose time is up leaves on the next removal run from now, not "any day now" (a run that
// already passed didn't take it).
func TestLeaveAt(t *testing.T) {
	runs, err := jobs.Parse("0 4 1 * *") // 4 AM on the 1st
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.Local)
	added := time.Date(2026, 8, 20, 0, 0, 0, 0, time.Local)
	if got := leaveAt(added, 30, runs, now); !got.Equal(time.Date(2026, 11, 1, 4, 0, 0, 0, time.Local)) {
		t.Errorf("overdue: %v", got)
	}
	added = time.Date(2026, 10, 10, 0, 0, 0, 0, time.Local)
	if got := leaveAt(added, 30, runs, now); !got.Equal(time.Date(2026, 12, 1, 4, 0, 0, 0, time.Local)) {
		t.Errorf("not due yet: %v", got)
	}
	if got := leaveAt(added, 30, nil, now); !got.Equal(added.Add(30 * 24 * time.Hour)) {
		t.Errorf("no schedule: %v", got)
	}
}
