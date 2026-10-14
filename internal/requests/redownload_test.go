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

// arrLog is a fake Radarr/Sonarr for redownloads that records what was asked of it.
type arrLog struct {
	mu    sync.Mutex
	calls []string
}

func (a *arrLog) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	p := strings.TrimPrefix(r.URL.Path, "/api/v3/")
	body, _ := io.ReadAll(r.Body)
	a.calls = append(a.calls, r.Method+" "+p+" "+string(body))
	reply := map[string]string{
		"movie":                 `[{"id":5,"hasFile":true,"movieFile":{"id":50}}]`,
		"movie/5":               `{"id":5,"monitored":false}`,
		"history/movie":         `[{"id":9,"eventType":"downloadFolderImported","downloadId":"A"},{"id":8,"eventType":"grabbed","downloadId":"A"},{"id":3,"eventType":"grabbed","downloadId":"OLD"}]`,
		"config/downloadclient": `{"autoRedownloadFailed":true}`,
		"series/lookup":         `[{"id":7}]`,
		"episode": `[{"id":71,"seasonNumber":1,"episodeFileId":701},{"id":72,"seasonNumber":1,"episodeFileId":701},
			{"id":81,"seasonNumber":2,"episodeFileId":801}]`,
		"history/series": `[{"id":20,"eventType":"downloadFolderImported","downloadId":"S2","episodeId":81},
			{"id":19,"eventType":"downloadFolderImported","downloadId":"S1","episodeId":71},{"id":18,"eventType":"grabbed","downloadId":"S1","episodeId":71},
			{"id":17,"eventType":"grabbed","downloadId":"S1","episodeId":72},{"id":16,"eventType":"grabbed","downloadId":"S2","episodeId":81}]`,
		"series/7": `{"id":7,"monitored":true,"seasons":[{"seasonNumber":1,"monitored":false},{"seasonNumber":2,"monitored":true}]}`,
	}[p]
	if reply == "" {
		reply = "{}"
	}
	w.Write([]byte(reply))
}

// at is where the first call starting with prefix happened (-1 = never).
func (a *arrLog) at(prefix string) int {
	for i, c := range a.calls {
		if strings.HasPrefix(c, prefix) {
			return i
		}
	}
	return -1
}

func (a *arrLog) did(prefix string) []string {
	out := []string{}
	for _, c := range a.calls {
		if strings.HasPrefix(c, prefix) {
			out = append(out, c)
		}
	}
	return out
}

func TestRedownload(t *testing.T) {
	dir := t.TempDir()
	box, _ := secret.Load(dir)
	st, _ := store.Open(dir, box)
	st.CreateUser(store.User{PlexID: 1, Username: "admin", Role: "Admin"})
	t.Cleanup(func() { st.Close() })
	radarr, sonarr := &arrLog{}, &arrLog{}
	rs, ss := httptest.NewServer(radarr), httptest.NewServer(sonarr)
	t.Cleanup(rs.Close)
	t.Cleanup(ss.Close)
	st.SaveServer(store.Server{Kind: "radarr", Name: "Radarr", URL: rs.URL, APIKey: "k", Enabled: true})
	st.SaveServer(store.Server{Kind: "sonarr", Name: "Sonarr", URL: ss.URL, APIKey: "k", Enabled: true})
	s := &Service{Store: st, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	ctx := context.Background()

	done, err := s.Redownload(ctx, "movie", 603, nil, For{UserID: 1, Title: "The Matrix"})
	if err != nil || !strings.Contains(done, "won't be grabbed again") {
		t.Fatalf("movie: %q %v", done, err)
	}
	// Only the grab the file came from is marked failed (not the older one); Radarr searches by itself.
	if f := radarr.did("POST history/failed/"); len(f) != 1 || !strings.HasPrefix(f[0], "POST history/failed/8") {
		t.Fatalf("marked: %v", f)
	}
	if len(radarr.did("DELETE moviefile/50")) != 1 || len(radarr.did("POST command")) != 0 {
		t.Fatalf("calls: %v", radarr.calls)
	}
	// Nobody had requested it: the new download gets its own request to follow.
	if list, _ := st.OpenRequests("movie", 603); len(list) != 1 || list[0].Source != "redownload" || list[0].ArrID != 5 || list[0].Status != "sent" {
		t.Fatalf("redownload request: %+v", list)
	}
	if ev, _ := st.RequestEvents(mustOpen(t, st, "movie", 603).ID); len(ev) < 2 || ev[0].Status != "redownload" || ev[1].Status != "sent" {
		t.Fatalf("timeline: %+v", ev)
	}
	// Delete, then monitor again (Radarr unmonitors on delete), then mark failed.
	if d, m, f := radarr.at("DELETE moviefile/50"), radarr.at("PUT movie/5"), radarr.at("POST history/failed/"); !(d >= 0 && d < m && m < f) {
		t.Fatalf("order: delete %d, monitor %d, failed %d", d, m, f)
	}

	// Season 1 only: its one pack is marked failed once, its file deleted, the season turned on.
	done, err = s.Redownload(ctx, "series", 1396, []int{1}, For{UserID: 1, Title: "Breaking Bad"})
	if err != nil || !strings.Contains(done, "Deleted 1 file.") {
		t.Fatalf("series: %q %v", done, err)
	}
	if f := sonarr.did("POST history/failed/"); len(f) != 1 || !strings.HasPrefix(f[0], "POST history/failed/18") {
		t.Fatalf("marked: %v", f)
	}
	var del map[string][]int
	d := sonarr.did("DELETE episodefile/bulk")
	if len(d) != 1 || json.Unmarshal([]byte(strings.TrimPrefix(d[0], "DELETE episodefile/bulk ")), &del) != nil || len(del["episodeFileIds"]) != 1 || del["episodeFileIds"][0] != 701 {
		t.Fatalf("deleted: %v", d)
	}
	if len(sonarr.did("PUT series/7")) != 1 {
		t.Fatalf("season 1 not turned on: %v", sonarr.calls)
	}
	if d, m, f := sonarr.at("DELETE episodefile/bulk"), sonarr.at("PUT episode/monitor"), sonarr.at("POST history/failed/"); !(d >= 0 && d < m && m < f) {
		t.Fatalf("order: delete %d, monitor %d, failed %d", d, m, f)
	}
	if mon := sonarr.did("PUT episode/monitor"); !strings.Contains(mon[0], "[71,72]") {
		t.Fatalf("episodes turned on: %v", mon)
	}
}

func mustOpen(t *testing.T, st *store.Store, typ string, id int) store.Request {
	t.Helper()
	list, _ := st.OpenRequests(typ, id)
	if len(list) == 0 {
		t.Fatalf("no request for %s:%d", typ, id)
	}
	return list[0]
}
