package requests

import (
	"context"
	"encoding/json"
	"errors"
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

// fakeArr answers the few Radarr/Sonarr calls requests make, and records what was added.
type fakeArr struct {
	mu      sync.Mutex
	added   []map[string]any
	hasFile bool
	queue   string // JSON for queue/details
	reject  bool   // answer adds with a validation error
	inLib   bool   // the title is already in the library
	put     map[string]any
	cmds    []map[string]any
}

func (f *fakeArr) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p := strings.TrimPrefix(r.URL.Path, "/api/v3/")
	id := 0
	if f.inLib {
		id = 77
	}
	switch {
	case p == "movie" && r.Method == http.MethodGet:
		if f.inLib {
			json.NewEncoder(w).Encode([]any{map[string]any{"id": 77, "tmdbId": 603, "hasFile": true}})
		} else {
			w.Write([]byte(`[]`))
		}
	case p == "movie/lookup/tmdb":
		json.NewEncoder(w).Encode(map[string]any{"title": "The Matrix", "tmdbId": 603, "year": 1999, "images": []any{}}) // Radarr 6: no id here even when it's in the library
	case p == "series/lookup":
		if r.URL.Query().Get("term") != "tmdb:1396" {
			w.Write([]byte(`[]`))
			return
		}
		json.NewEncoder(w).Encode([]any{map[string]any{"title": "Breaking Bad", "tvdbId": 81189, "id": id,
			"seasons": []any{map[string]any{"seasonNumber": 0, "monitored": true}, map[string]any{"seasonNumber": 1, "monitored": true}, map[string]any{"seasonNumber": 2, "monitored": true}, map[string]any{"seasonNumber": 3, "monitored": true}}}})
	case (p == "movie" || p == "series") && r.Method == http.MethodPost:
		if f.reject {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`[{"propertyName":"Path","errorMessage":"Path is already configured for an existing movie"}]`))
			return
		}
		var body map[string]any
		b, _ := io.ReadAll(r.Body)
		json.Unmarshal(b, &body)
		f.added = append(f.added, body)
		body["id"] = 42
		json.NewEncoder(w).Encode(body)
	case p == "movie/42":
		json.NewEncoder(w).Encode(map[string]any{"id": 42, "hasFile": f.hasFile})
	case p == "series/42":
		have := 0
		if f.hasFile {
			have = 8
		}
		json.NewEncoder(w).Encode(map[string]any{"id": 42, "seasons": []any{
			map[string]any{"seasonNumber": 1, "statistics": map[string]any{"episodeFileCount": have, "episodeCount": 8}},
			map[string]any{"seasonNumber": 2, "statistics": map[string]any{"episodeFileCount": have, "episodeCount": 8}},
			map[string]any{"seasonNumber": 3, "statistics": map[string]any{"episodeFileCount": 0, "episodeCount": 8}}}})
	case p == "series/77" && r.Method == http.MethodGet:
		json.NewEncoder(w).Encode(map[string]any{"id": 77, "monitored": true, "seasons": []any{
			map[string]any{"seasonNumber": 0, "monitored": false}, map[string]any{"seasonNumber": 1, "monitored": true}, map[string]any{"seasonNumber": 2, "monitored": false}, map[string]any{"seasonNumber": 3, "monitored": false}}})
	case p == "series/77" && r.Method == http.MethodPut:
		b, _ := io.ReadAll(r.Body)
		json.Unmarshal(b, &f.put)
		w.Write(b)
	case p == "command":
		var body map[string]any
		b, _ := io.ReadAll(r.Body)
		json.Unmarshal(b, &body)
		f.cmds = append(f.cmds, body)
		w.Write([]byte(`{}`))
	case p == "queue/details":
		q := f.queue
		if q == "" {
			q = "[]"
		}
		w.Write([]byte(q))
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func setup(t *testing.T) (*Service, *store.Store, *fakeArr, *fakeArr, *store.User, *store.User) {
	dir := t.TempDir()
	box, _ := secret.Load(dir)
	st, err := store.Open(dir, box)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	radarr, sonarr := &fakeArr{}, &fakeArr{}
	rs, ss := httptest.NewServer(radarr), httptest.NewServer(sonarr)
	t.Cleanup(rs.Close)
	t.Cleanup(ss.Close)
	st.SaveServer(store.Server{Kind: "radarr", Name: "Radarr", URL: rs.URL, APIKey: "k", RootFolder: "/movies", QualityProfileID: 27, QualityProfileName: "Flixarr · Standard", SearchOnAdd: true, Enabled: true})
	st.SaveServer(store.Server{Kind: "sonarr", Name: "Sonarr", URL: ss.URL, APIKey: "k", RootFolder: "/tv", QualityProfileID: 8, QualityProfileName: "Flixarr · Standard", SeasonFolders: true, SearchOnAdd: false, Enabled: true})
	admin, _ := st.CreateUser(store.User{PlexID: 1, Username: "admin", Role: "Admin"})
	user, _ := st.CreateUser(store.User{PlexID: 2, Username: "eryn", Role: "User"})
	return &Service{Store: st, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}, st, radarr, sonarr, admin, user
}

func TestMovieRequestFlow(t *testing.T) {
	s, st, radarr, _, admin, _ := setup(t)
	ctx := context.Background()
	r, err := s.Create(ctx, admin, New{Type: "movie", TmdbID: 603, Title: "The Matrix", Year: "1999"})
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "sent" || r.ArrID != 42 || len(radarr.added) != 1 {
		t.Fatalf("after create: %+v added %d", r, len(radarr.added))
	}
	a := radarr.added[0]
	if a["qualityProfileId"].(float64) != 27 || a["rootFolderPath"] != "/movies" || a["minimumAvailability"] != "released" || a["addOptions"].(map[string]any)["searchForMovie"] != true {
		t.Fatalf("sent to Radarr: %v", a)
	}
	if _, err := s.Create(ctx, admin, New{Type: "movie", TmdbID: 603, Title: "The Matrix"}); err != ErrAlready {
		t.Fatalf("second request: %v", err)
	}
	// In the queue: downloading with progress. Then the file is there: available.
	radarr.queue = `[{"size":1000,"sizeleft":250}]`
	s.Refresh(ctx)
	r, _ = st.Request(r.ID)
	if r.Status != "downloading" || r.Progress != 75 {
		t.Fatalf("downloading: %+v", r)
	}
	radarr.queue, radarr.hasFile = "", true
	s.Refresh(ctx)
	r, _ = st.Request(r.ID)
	if r.Status != "available" {
		t.Fatalf("available: %+v", r)
	}
}

func TestSeriesSeasonsAndAnime(t *testing.T) {
	s, st, _, sonarr, admin, _ := setup(t)
	ctx := context.Background()
	r, err := s.Create(ctx, admin, New{Type: "series", TmdbID: 1396, Title: "Breaking Bad", Seasons: []int{1, 2}, Genres: []string{"Animation", "Drama"}, Language: "ja"})
	if err != nil || r.Status != "sent" {
		t.Fatalf("create: %+v %v", r, err)
	}
	a := sonarr.added[0]
	mon := map[int]bool{}
	for _, x := range a["seasons"].([]any) {
		m := x.(map[string]any)
		mon[int(m["seasonNumber"].(float64))] = m["monitored"].(bool)
	}
	if mon[0] || !mon[1] || !mon[2] || mon[3] || a["seriesType"] != "anime" || a["seasonFolder"] != true || a["addOptions"].(map[string]any)["searchForMissingEpisodes"] != false {
		t.Fatalf("sent to Sonarr: seasons %v type %v %v", mon, a["seriesType"], a["addOptions"])
	}
	// Seasons 1 and 2 complete (3 wasn't requested): available.
	sonarr.hasFile = true
	s.Refresh(ctx)
	if r, _ = st.Request(r.ID); r.Status != "available" {
		t.Fatalf("series available: %+v", r)
	}
}

func TestApprovalAlreadyThereAndFailure(t *testing.T) {
	s, st, radarr, _, admin, user := setup(t)
	ctx := context.Background()
	st.SetSetting(KeyApproval, "true")
	// A non-admin waits for approval; nothing is sent yet.
	r, err := s.Create(ctx, user, New{Type: "movie", TmdbID: 603, Title: "The Matrix"})
	if err != nil || r.Status != "pending" || len(radarr.added) != 0 {
		t.Fatalf("pending: %+v %v", r, err)
	}
	// Already in Radarr with its file: linked and available, not added again.
	radarr.inLib = true
	r = s.Send(ctx, r)
	if r.Status != "available" || r.ArrID != 77 || len(radarr.added) != 0 {
		t.Fatalf("already there: %+v", r)
	}
	// Radarr refuses: failed, with its own message.
	st.DeleteRequest(r.ID)
	radarr.inLib, radarr.reject = false, true
	r, _ = s.Create(ctx, admin, New{Type: "movie", TmdbID: 603, Title: "The Matrix"})
	if r.Status != "failed" || !strings.Contains(r.Error, "already configured") {
		t.Fatalf("failed: %+v", r)
	}
	// A failed request can be made again.
	radarr.reject = false
	if r2, err := s.Create(ctx, admin, New{Type: "movie", TmdbID: 603, Title: "The Matrix"}); err != nil || r2.Status != "sent" {
		t.Fatalf("again after failure: %+v %v", r2, err)
	}
}

func TestPlexMakesAvailable(t *testing.T) {
	s, st, _, _, admin, _ := setup(t)
	ctx := context.Background()
	r, _ := s.Create(ctx, admin, New{Type: "movie", TmdbID: 603, Title: "The Matrix"})
	st.SavePlexItem(store.PlexItem{Type: "movie", TmdbID: 603, RatingKey: "1", Section: "1", SeenAt: "2026-01-01T00:00:00Z"})
	s.Refresh(ctx)
	if r, _ = st.Request(r.ID); r.Status != "available" {
		t.Fatalf("in Plex: %+v", r)
	}
	st.DeleteRequest(r.ID)
	if _, err := s.Create(ctx, admin, New{Type: "movie", TmdbID: 603, Title: "The Matrix"}); err != ErrInPlex {
		t.Fatalf("request something in Plex: %v", err)
	}
}

func TestSeriesInPlexStillRequestable(t *testing.T) {
	s, st, _, _, admin, _ := setup(t)
	ctx := context.Background()
	st.SavePlexItem(store.PlexItem{Type: "series", TmdbID: 1396, RatingKey: "7", Section: "2", SeenAt: "2026-01-01T00:00:00Z"})
	r, err := s.Create(ctx, admin, New{Type: "series", TmdbID: 1396, Title: "Breaking Bad", Seasons: []int{3}})
	if err != nil || r.Status != "sent" {
		t.Fatalf("series in Plex: %+v %v", r, err)
	}
	s.Refresh(ctx) // season 3 has no files: not available just because the show is in Plex
	if r, _ = st.Request(r.ID); r.Status == "available" {
		t.Fatalf("series marked available from Plex: %+v", r)
	}
}

func TestMoreSeasonsOfSeriesInSonarr(t *testing.T) {
	s, st, _, sonarr, admin, _ := setup(t)
	ctx := context.Background()
	sonarr.inLib = true
	srv, _, _ := st.FirstServer("sonarr")
	srv.SearchOnAdd = true
	st.SaveServer(srv)
	r, err := s.Create(ctx, admin, New{Type: "series", TmdbID: 1396, Title: "Breaking Bad", Seasons: []int{3}})
	if err != nil || r.Status != "sent" || r.ArrID != 77 || len(sonarr.added) != 0 {
		t.Fatalf("create: %+v %v", r, err)
	}
	mon := map[int]bool{}
	for _, x := range sonarr.put["seasons"].([]any) {
		m := x.(map[string]any)
		mon[int(m["seasonNumber"].(float64))] = m["monitored"].(bool)
	}
	if !mon[1] || mon[2] || !mon[3] || mon[0] {
		t.Fatalf("monitored after request: %v", mon)
	}
	if len(sonarr.cmds) != 1 || sonarr.cmds[0]["name"] != "SeasonSearch" || sonarr.cmds[0]["seasonNumber"].(float64) != 3 {
		t.Fatalf("search commands: %v", sonarr.cmds)
	}
}

func TestPermissions(t *testing.T) {
	s, st, radarr, _, _, user := setup(t)
	ctx := context.Background()
	st.SetSetting(KeyApproval, "true")
	// No "request movies": refused.
	u, _ := st.SetPerms(user.ID, []string{"requestTv"})
	if _, err := s.Create(ctx, u, New{Type: "movie", TmdbID: 603, Title: "The Matrix"}); err != ErrNotAllowed {
		t.Fatalf("without requestMovies: %v", err)
	}
	// Auto-approve skips approval even though approval is on; a chosen quality is ignored without "advanced".
	u, _ = st.SetPerms(user.ID, []string{"requestMovies", "autoApprove"})
	if u.Role != "Custom" {
		t.Fatalf("role label: %s", u.Role)
	}
	r, err := s.Create(ctx, u, New{Type: "movie", TmdbID: 603, Title: "The Matrix", ProfileID: 99})
	if err != nil || r.Status != "sent" || r.ProfileID != 27 || len(radarr.added) != 1 {
		t.Fatalf("auto-approved: %+v %v", r, err)
	}
	if _, err := st.SetPerms(1, []string{"requestMovies"}); err != store.ErrOwner {
		t.Fatalf("owner losing admin: %v", err)
	}
	if err := st.DeleteUser(1); err != store.ErrOwner {
		t.Fatalf("owner removed: %v", err)
	}
	if u, _ = st.SetPerms(user.ID, store.RolePerms["Trusted"]); u.Role != "Trusted" {
		t.Fatalf("preset label: %s", u.Role)
	}
}

func TestLimits(t *testing.T) {
	s, st, _, _, _, user := setup(t)
	ctx := context.Background()
	st.SetLimits(user.ID, 1, 3)
	u, _ := st.UserByID(user.ID)
	if _, err := s.Create(ctx, u, New{Type: "movie", TmdbID: 603, Title: "The Matrix"}); err != nil {
		t.Fatal(err)
	}
	var le LimitError
	if _, err := s.Create(ctx, u, New{Type: "movie", TmdbID: 604, Title: "Reloaded"}); !errors.As(err, &le) {
		t.Fatalf("second movie: %v", err)
	}
	// Whole show of 4 seasons is over 3; two seasons fit.
	if _, err := s.Create(ctx, u, New{Type: "series", TmdbID: 1396, Title: "Breaking Bad", SeasonCount: 4}); !errors.As(err, &le) {
		t.Fatalf("whole show over the limit: %v", err)
	}
	if _, err := s.Create(ctx, u, New{Type: "series", TmdbID: 1396, Title: "Breaking Bad", Seasons: []int{1, 2}}); err != nil {
		t.Fatalf("two seasons: %v", err)
	}
	if got := s.UsageFor(u, "series"); got.Used != 2 || got.Limit != 3 {
		t.Fatalf("usage: %+v", got)
	}
}
