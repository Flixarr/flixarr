package requests

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/Flixarr/flixarr/internal/secret"
	"github.com/Flixarr/flixarr/internal/store"
)

// A Sonarr with one stuck download (queue item 41, series 9).
type stuckQueue struct {
	mu    sync.Mutex
	calls []string
}

func (q *stuckQueue) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	q.mu.Lock()
	q.calls = append(q.calls, r.Method+" "+strings.TrimPrefix(r.URL.Path, "/api/v3/")+"?"+r.URL.RawQuery+" "+string(b))
	q.mu.Unlock()
	if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/queue") {
		json.NewEncoder(w).Encode(map[string]any{"records": []map[string]any{{"id": 41, "seriesId": 9, "outputPath": "/downloads/Jackass.S01E02", "downloadId": "SAB_1", "series": map[string]any{"tmdbId": 1795}}}})
		return
	}
	w.Write([]byte("{}"))
}

func TestFixDownload(t *testing.T) {
	dir := t.TempDir()
	box, _ := secret.Load(dir)
	st, _ := store.Open(dir, box)
	t.Cleanup(func() { st.Close() })
	q := &stuckQueue{}
	srv := httptest.NewServer(q)
	t.Cleanup(srv.Close)
	st.SaveServer(store.Server{Kind: "sonarr", Name: "Sonarr", URL: srv.URL, APIKey: "k", Enabled: true})
	s := &Service{Store: st, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	ctx := context.Background()

	last := func() string { q.mu.Lock(); defer q.mu.Unlock(); return q.calls[len(q.calls)-1] }
	f, err := s.FixDownload(ctx, "sonarr", 41, "different")
	if err != nil || f.Type != "series" || f.ArrID != 9 || f.TmdbID != 1795 {
		t.Fatalf("different: %+v %v", f, err)
	}
	if c := last(); !strings.HasPrefix(c, "DELETE queue/41?") || !strings.Contains(c, "blocklist=true") || !strings.Contains(c, "skipRedownload=false") || !strings.Contains(c, "removeFromClient=true") {
		t.Errorf("different: %s", c)
	}
	s.FixDownload(ctx, "sonarr", 41, "remove")
	if c := last(); !strings.Contains(c, "blocklist=false") || !strings.Contains(c, "skipRedownload=true") {
		t.Errorf("remove: %s", c)
	}
	s.FixDownload(ctx, "sonarr", 41, "retry")
	if c := last(); !strings.HasPrefix(c, "POST command") || !strings.Contains(c, `"DownloadedEpisodesScan"`) || !strings.Contains(c, "/downloads/Jackass.S01E02") || !strings.Contains(c, "SAB_1") {
		t.Errorf("retry: %s", c)
	}
	if _, err := s.FixDownload(ctx, "sonarr", 99, "different"); err == nil || !strings.Contains(err.Error(), "isn't in the queue") {
		t.Errorf("gone: %v", err)
	}
	if _, err := s.FixDownload(ctx, "sonarr", 41, "explode"); err == nil {
		t.Error("unknown action accepted")
	}
	if _, err := s.FixDownload(ctx, "radarr", 41, "different"); err == nil {
		t.Error("Radarr isn't connected, but it worked")
	}
}
