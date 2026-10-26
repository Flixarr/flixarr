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
	"time"

	"github.com/Flixarr/flixarr/internal/arr"
	"github.com/Flixarr/flixarr/internal/plexlib"
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
	monitor map[string]any
	epFile  bool
	noFile  bool // in the library without a file
	gone    bool // movie 42 was deleted from Radarr
	files   int  // series 42: episodes with files in seasons 1 and 2 (when hasFile is off)
	deleted []string
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
	case r.Method == http.MethodDelete && p == "episodefile/bulk":
		b, _ := io.ReadAll(r.Body)
		f.deleted = append(f.deleted, p+" "+string(b))
	case p == "history/series":
		w.Write([]byte(`[]`))
	case r.Method == http.MethodDelete && (p == "movie/77" || p == "series/77"):
		f.deleted = append(f.deleted, p+"?"+r.URL.RawQuery)
	case p == "movie" && r.Method == http.MethodGet:
		if f.inLib {
			json.NewEncoder(w).Encode([]any{map[string]any{"id": 77, "tmdbId": 603, "hasFile": !f.noFile, "monitored": false}})
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
		json.NewEncoder(w).Encode([]any{map[string]any{"title": "Breaking Bad", "tmdbId": 1396, "tvdbId": 81189, "id": id,
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
	case p == "movie/42" && f.gone:
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"title":"Not Found","status":404}`))
	case p == "movie/42":
		json.NewEncoder(w).Encode(map[string]any{"id": 42, "hasFile": f.hasFile})
	case p == "series/42":
		have := f.files
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
	case p == "movie/77" && r.Method == http.MethodGet:
		json.NewEncoder(w).Encode(map[string]any{"id": 77, "tmdbId": 603, "hasFile": !f.noFile, "monitored": false, "titleSlug": "the-matrix-1999",
			"movieFile": map[string]any{"size": 1000, "sceneName": "The.Matrix.1999.1080p", "quality": map[string]any{"quality": map[string]any{"name": "Bluray-1080p"}}}})
	case p == "movie/77" && r.Method == http.MethodPut:
		b, _ := io.ReadAll(r.Body)
		json.Unmarshal(b, &f.put)
		w.Write(b)
	case p == "series/77" && r.Method == http.MethodPut:
		b, _ := io.ReadAll(r.Body)
		json.Unmarshal(b, &f.put)
		w.Write(b)
	case p == "episode" && r.Method == http.MethodGet:
		json.NewEncoder(w).Encode([]any{
			map[string]any{"id": 501, "seasonNumber": 1, "episodeNumber": 1, "hasFile": f.epFile, "episodeFileId": map[bool]int{true: 9001}[f.epFile]},
			map[string]any{"id": 502, "seasonNumber": 1, "episodeNumber": 2, "hasFile": false},
			map[string]any{"id": 503, "seasonNumber": 2, "episodeNumber": 1, "hasFile": f.epFile}})
	case p == "episode/monitor":
		b, _ := io.ReadAll(r.Body)
		json.Unmarshal(b, &f.monitor)
		w.Write([]byte(`[]`))
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
	s.checked = nil
	s.Refresh(ctx)
	r, _ = st.Request(r.ID)
	if r.Status != "downloading" || r.Progress != 75 {
		t.Fatalf("downloading: %+v", r)
	}
	radarr.queue, radarr.hasFile = "", true
	s.checked = nil
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
	s.checked = nil
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
	s.checked = nil
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

func TestBlocked(t *testing.T) {
	s, st, _, _, admin, _ := setup(t)
	st.Block(store.Blocked{Type: "movie", TmdbID: 603, Title: "The Matrix"}, admin.ID)
	if _, err := s.Create(context.Background(), admin, New{Type: "movie", TmdbID: 603, Title: "The Matrix"}); err != ErrBlocked {
		t.Fatalf("blocked: %v", err)
	}
	st.Unblock("movie", 603)
	if _, err := s.Create(context.Background(), admin, New{Type: "movie", TmdbID: 603, Title: "The Matrix"}); err != nil {
		t.Fatalf("unblocked: %v", err)
	}
}

type fakePlex struct {
	rescans int
	has     bool
	st      *store.Store
}

func (f *fakePlex) Connected() bool                                      { return true }
func (f *fakePlex) Live() bool                                           { return false }
func (f *fakePlex) Seasons(context.Context, int) (map[int]int, error)    { return map[int]int{}, nil }
func (f *fakePlex) Rescan(context.Context, string) error                 { f.rescans++; return nil }
func (f *fakePlex) FileItem(string) (plexlib.FileItem, bool)             { return plexlib.FileItem{}, false }
func (f *fakePlex) FixMatch(context.Context, string, int) error          { return nil }
func (f *fakePlex) OrderMismatch(context.Context, int, map[int]int) bool { return false }
func (f *fakePlex) UseSonarrOrder(context.Context, int) error            { return nil }
func (f *fakePlex) RecentScan(context.Context) (string, error) {
	if f.has {
		f.st.SavePlexItem(store.PlexItem{Type: "movie", TmdbID: 603, RatingKey: "9", Section: "1", SeenAt: "x"})
	}
	return "", nil
}

func TestStatesGrabbedImportingPlex(t *testing.T) {
	s, st, radarr, _, admin, _ := setup(t)
	px := &fakePlex{st: st}
	s.Plex = px
	ctx := context.Background()
	r, _ := s.Create(ctx, admin, New{Type: "movie", TmdbID: 603, Title: "The Matrix"})
	step := func(want string) {
		t.Helper()
		s.checked = nil // each step is a later check
		s.checked = nil
		s.Refresh(ctx)
		if r, _ = st.Request(r.ID); r.Status != want {
			t.Fatalf("want %s, got %+v", want, r)
		}
	}
	radarr.queue = `[{"size":1000,"sizeleft":1000,"status":"queued","trackedDownloadState":"downloading"}]`
	step("grabbed")
	radarr.queue = `[{"size":1000,"sizeleft":400,"status":"downloading","trackedDownloadState":"downloading"}]`
	step("downloading")
	radarr.queue = `[{"size":1000,"sizeleft":0,"status":"completed","trackedDownloadState":"importPending"}]`
	step("importing")
	// Radarr has the file but Plex doesn't yet: still importing, and Plex is asked to scan.
	radarr.queue, radarr.hasFile = "", true
	step("importing")
	if px.rescans != 1 {
		t.Fatalf("rescans: %d", px.rescans)
	}
	px.has = true
	s.FollowImporting(ctx) // what a Plex "added" message does
	if r, _ = st.Request(r.ID); r.Status != "available" {
		t.Fatalf("after Plex added it: %+v", r)
	}
}

func TestRequestAlerts(t *testing.T) {
	s, st, _, _, admin, user := setup(t)
	ctx := context.Background()
	st.SetSetting(KeyApproval, "true")
	r, _ := s.Create(ctx, user, New{Type: "movie", TmdbID: 603, Title: "The Matrix"})
	bell := func(id int64, can func(string) bool) []store.Alert { l, _ := st.Bell(id, can); return l }
	if a := bell(admin.ID, store.CanAll); len(a) != 1 || a[0].Kind != "todo" || a[0].Title != "eryn requested" {
		t.Fatalf("admin to-do: %+v", a)
	}
	if a := bell(user.ID, store.CanNone); len(a) != 0 {
		t.Fatalf("nothing for the user yet: %+v", a)
	}
	// An admin approves: the to-do goes; the user hears Approved (and the request is sent).
	st.DecideRequest(r.ID, admin.ID, "approved")
	r.Status = "approved"
	r = s.Send(ctx, r)
	s.Decided(r, admin)
	if a := bell(admin.ID, store.CanAll); len(a) != 0 {
		t.Fatalf("admin to-do should be gone: %+v", a)
	}
	if a := bell(user.ID, store.CanNone); len(a) != 1 || a[0].Title != "Approved" {
		t.Fatalf("user hears approved: %+v", a)
	}
	// In Plex: Ready to watch, its own alert. Ready again later: Ready to watch again.
	r.Status = "available"
	st.UpdateRequest(r)
	s.Alert(r)
	got := bell(user.ID, store.CanNone)
	if len(got) != 2 || got[0].Title != "Ready to watch" || got[0].Detail != "The Matrix" || !got[0].New {
		t.Fatalf("ready: %+v", got)
	}
	time.Sleep(1100 * time.Millisecond)
	s.Alert(r)
	if got := bell(user.ID, store.CanNone); got[0].Title != "Ready to watch again" {
		t.Fatalf("again: %+v", got[0])
	}
	// An admin approving their own request is no news to them.
	mine, _ := s.Create(ctx, admin, New{Type: "movie", TmdbID: 604, Title: "Reloaded"})
	s.Decided(mine, admin)
	for _, a := range bell(admin.ID, store.CanAll) {
		if a.Title == "Approved" {
			t.Fatal("an admin was told they approved their own request")
		}
	}
	// A failed request: the user hears it and admins get a to-do, gone when it's retried.
	f, _ := st.CreateRequest(store.Request{Type: "movie", TmdbID: 605, Title: "Revolutions", UserID: user.ID, Status: "failed", Error: "no release"})
	s.Alert(f)
	if a := bell(user.ID, store.CanNone); a[0].Title != "Couldn't get it" {
		t.Fatalf("user failed: %+v", a[0])
	}
	if a := bell(admin.ID, store.CanAll); len(a) != 1 || a[0].Title != "Request failed" {
		t.Fatalf("admin failed: %+v", a)
	}
	f.Status = "sent"
	s.Alert(f)
	if a := bell(admin.ID, store.CanAll); len(a) != 0 {
		t.Fatalf("retried, to-do should be gone: %+v", a)
	}
}

func TestEpisodeRequests(t *testing.T) {
	s, st, _, sonarr, admin, _ := setup(t)
	ctx := context.Background()
	s.SetModes(TVModes{Whole: true, Seasons: true, Episodes: true, Default: "whole"})
	r, err := s.Create(ctx, admin, New{Type: "series", TmdbID: 1396, Title: "Breaking Bad", Episodes: [][2]int{{1, 1}, {2, 1}}})
	if err != nil || r.Status != "sent" || r.Mode != "episodes" {
		t.Fatalf("create: %+v %v", r, err)
	}
	if opts := sonarr.added[0]["addOptions"].(map[string]any); opts["monitor"] != "none" {
		t.Fatalf("add options: %v", opts)
	}
	ids := sonarr.monitor["episodeIds"].([]any)
	if len(ids) != 2 || ids[0].(float64) != 501 || ids[1].(float64) != 503 {
		t.Fatalf("monitored: %v", sonarr.monitor)
	}
	sonarr.epFile = true
	s.checked = nil
	s.Refresh(ctx)
	if r, _ = st.Request(r.ID); r.Status != "available" {
		t.Fatalf("episodes with files: %+v", r)
	}
	// Turned off in Settings: refused.
	s.SetModes(TVModes{Whole: true})
	if _, err := s.Create(ctx, admin, New{Type: "series", TmdbID: 1399, Title: "Other", Episodes: [][2]int{{1, 1}}}); err == nil {
		t.Fatal("episode request allowed while turned off")
	}
}

func TestMovieInRadarrWithoutFile(t *testing.T) {
	s, st, radarr, _, admin, _ := setup(t)
	radarr.inLib, radarr.noFile = true, true
	r, err := s.Create(context.Background(), admin, New{Type: "movie", TmdbID: 603, Title: "The Matrix"})
	if err != nil || r.Status != "sent" || r.ArrID != 77 {
		t.Fatalf("create: %+v %v", r, err)
	}
	if radarr.put["monitored"] != true || len(radarr.cmds) != 1 || radarr.cmds[0]["name"] != "MoviesSearch" {
		t.Fatalf("monitor + search: put %v cmds %v", radarr.put, radarr.cmds)
	}
	_ = st
}

func TestRecheckAfterPlexRemoval(t *testing.T) {
	s, st, radarr, _, admin, user := setup(t)
	px := &fakePlex{st: st}
	s.Plex = px
	ctx := context.Background()
	r, _ := s.Create(ctx, user, New{Type: "movie", TmdbID: 603, Title: "The Matrix"})
	st.SavePlexItem(store.PlexItem{Type: "movie", TmdbID: 603, RatingKey: "9", Section: "1", SeenAt: "x"})
	radarr.hasFile = true
	s.Refresh(ctx)
	if r, _ = st.Request(r.ID); r.Status != "available" {
		t.Fatalf("setup: %+v", r)
	}
	// Still in Plex: nothing changes.
	s.Recheck(ctx, []string{"movie:603"}, false)
	if r, _ = st.Request(r.ID); r.Status != "available" {
		t.Fatalf("still in Plex: %+v", r)
	}
	// Gone from Plex, file still in Radarr: importing again, and Plex is asked to scan.
	st.RemovePlexItems([]string{"9"})
	s.Recheck(ctx, []string{"movie:603"}, false)
	if r, _ = st.Request(r.ID); r.Status != "importing" || px.rescans == 0 {
		t.Fatalf("file still in Radarr: %+v (rescans %d)", r, px.rescans)
	}
	// Back to available, then deleted from Radarr too: failed with the reason, and it can be asked for again.
	r.Status = "available"
	st.UpdateRequest(r)
	radarr.gone = true
	s.Recheck(ctx, []string{"movie:603"}, false)
	if r, _ = st.Request(r.ID); r.Status != "removed" || !strings.HasPrefix(r.Error, "Removed from Plex and Radarr") {
		t.Fatalf("gone everywhere: %+v", r)
	}
	if a, _ := st.Bell(user.ID, store.CanNone); len(a) == 0 || a[0].Title != "Removed" || !strings.HasPrefix(a[0].Detail, "The Matrix") {
		t.Fatalf("alert: %+v", a)
	}
	radarr.gone, radarr.hasFile = false, false
	if _, err := s.Create(ctx, admin, New{Type: "movie", TmdbID: 603, Title: "The Matrix"}); err != nil {
		t.Fatalf("request again: %v", err)
	}
}

func TestOpenRequestWhileGrabbed(t *testing.T) {
	s, st, _, _, admin, _ := setup(t)
	ctx := context.Background()
	r, _ := s.Create(ctx, admin, New{Type: "movie", TmdbID: 603, Title: "The Matrix"})
	for _, status := range []string{"grabbed", "importing"} {
		r.Status = status
		st.UpdateRequest(r)
		if _, err := s.Create(ctx, admin, New{Type: "movie", TmdbID: 603, Title: "The Matrix"}); err != ErrAlready {
			t.Fatalf("%s: asked twice: %v", status, err)
		}
	}
}

func TestSeriesMoreSeasons(t *testing.T) {
	s, st, _, sonarr, admin, _ := setup(t)
	ctx := context.Background()
	r1, err := s.Create(ctx, admin, New{Type: "series", TmdbID: 1396, Title: "Breaking Bad", Seasons: []int{1}})
	if err != nil || r1.Status != "sent" {
		t.Fatalf("season 1: %+v %v", r1, err)
	}
	if got := sonarr.added[0]["monitorNewItems"]; got != "none" {
		t.Fatalf("picked seasons follow later seasons: %v", got)
	}
	// Season 1 again: nothing new to ask for.
	if _, err := s.Create(ctx, admin, New{Type: "series", TmdbID: 1396, Title: "Breaking Bad", Seasons: []int{1}}); err != ErrAlready {
		t.Fatalf("season 1 twice: %v", err)
	}
	// Seasons 1 and 2: only season 2 is new, and it goes in its own request.
	r2, err := s.Create(ctx, admin, New{Type: "series", TmdbID: 1396, Title: "Breaking Bad", Seasons: []int{1, 2}})
	if err != nil || len(r2.Seasons) != 1 || r2.Seasons[0] != 2 || r2.ID == r1.ID {
		t.Fatalf("season 2: %+v %v", r2, err)
	}
	if c, _ := st.SeriesCoverage(1396); c.Whole || len(c.Seasons) != 2 {
		t.Fatalf("coverage: %+v", c)
	}
	// The whole show on top: allowed once, then nothing more.
	if _, err := s.Create(ctx, admin, New{Type: "series", TmdbID: 1396, Title: "Breaking Bad", SeasonCount: 5}); err != nil {
		t.Fatalf("whole show: %v", err)
	}
	// Seasons 1 and 2, then the whole show of 5: 5 seasons used, not 7.
	if _, used, _ := st.RecentUsage(admin.ID); used != 5 {
		t.Fatalf("seasons counted: %d", used)
	}
	if _, err := s.Create(ctx, admin, New{Type: "series", TmdbID: 1396, Title: "Breaking Bad", Seasons: []int{3}}); err != ErrAlready {
		t.Fatalf("after the whole show: %v", err)
	}
	// A finished request keeps the title's state from hiding one still on its way.
	r1.Status = "available"
	st.UpdateRequest(r1)
	if got, _ := st.RequestStatusFor([]string{"series:1396"}); got["series:1396"].Status == "available" {
		t.Fatalf("status hides active requests: %+v", got)
	}
}

func TestWholeShowFollowsNewSeasons(t *testing.T) {
	s, _, _, sonarr, admin, _ := setup(t)
	ctx := context.Background()
	if _, err := s.Create(ctx, admin, New{Type: "series", TmdbID: 1396, Title: "Breaking Bad"}); err != nil {
		t.Fatal(err)
	}
	if got := sonarr.added[0]["monitorNewItems"]; got != "all" {
		t.Fatalf("whole show: %v", got)
	}
	// Already in Sonarr: a whole-show request turns following on too.
	s2, _, _, sonarr2, admin2, _ := setup(t)
	sonarr2.inLib = true
	if _, err := s2.Create(ctx, admin2, New{Type: "series", TmdbID: 1396, Title: "Breaking Bad"}); err != nil {
		t.Fatal(err)
	}
	if sonarr2.put["monitorNewItems"] != "all" {
		t.Fatalf("existing series: %v", sonarr2.put["monitorNewItems"])
	}
}

func TestEpisodesOffUntilTurnedOn(t *testing.T) {
	s, _, _, _, admin, _ := setup(t)
	if m := s.Modes(); m.Episodes {
		t.Fatalf("episodes offered by default: %+v", m)
	}
	if _, err := s.Create(context.Background(), admin, New{Type: "series", TmdbID: 1396, Title: "Breaking Bad", Episodes: [][2]int{{1, 1}}}); err == nil {
		t.Fatal("episode request allowed while off")
	}
	s.SetModes(TVModes{Seasons: true, Episodes: true, Default: "episodes"})
	if m := s.Modes(); !m.Episodes || m.Default != "episodes" {
		t.Fatalf("turned on: %+v", m)
	}
}

func TestSeasonProgressCountsFinishedEpisodes(t *testing.T) {
	s, st, _, sonarr, admin, _ := setup(t)
	ctx := context.Background()
	r, _ := s.Create(ctx, admin, New{Type: "series", TmdbID: 1396, Title: "Breaking Bad", Seasons: []int{1}})
	check := func(status string, progress int) {
		t.Helper()
		s.checked = nil
		s.Refresh(ctx)
		if r, _ = st.Request(r.ID); r.Status != status || r.Progress != progress {
			t.Fatalf("want %s %d%%, got %s %d%%", status, progress, r.Status, r.Progress)
		}
	}
	// Only another season downloading: nothing for this request yet.
	sonarr.queue = `[{"seasonNumber":2,"size":1000,"sizeleft":500,"status":"downloading","trackedDownloadState":"downloading"}]`
	check("sent", 0)
	// One episode of 8 at half: 6%.
	sonarr.queue = `[{"seasonNumber":1,"size":1000,"sizeleft":500,"status":"downloading","trackedDownloadState":"downloading"},{"seasonNumber":2,"size":1000,"sizeleft":0,"status":"downloading","trackedDownloadState":"downloading"}]`
	check("downloading", 6)
	// That one imported, nothing in the queue yet for the rest: still downloading (1 of 8), not back to Requested.
	sonarr.queue, sonarr.files = "", 1
	check("downloading", 12)
	// Next one done and importing while 6 are still missing: not "Importing" for the whole season.
	sonarr.queue = `[{"seasonNumber":1,"size":1000,"sizeleft":0,"status":"completed","trackedDownloadState":"importPending"}]`
	check("downloading", 25)
	// The last one being moved in: Importing.
	sonarr.files = 7
	check("importing", 100)
}

func TestFix(t *testing.T) {
	s, st, radarr, sonarr, admin, _ := setup(t)
	ctx := context.Background()
	r, _ := s.Create(ctx, admin, New{Type: "movie", TmdbID: 603, Title: "The Matrix"})
	radarr.cmds = nil
	// In Radarr but not monitored: turned on, then searched.
	r, done, err := s.Fix(ctx, r)
	if err != nil || !strings.Contains(done, "monitored") || !strings.Contains(done, "Started a search") || len(radarr.cmds) != 1 {
		t.Fatalf("fix: %q %v cmds %v", done, err, radarr.cmds)
	}
	// Gone from Radarr: added again.
	radarr.gone = true
	before := len(radarr.added)
	if _, done, err = s.Fix(ctx, r); err != nil || !strings.Contains(done, "missing from Radarr") || len(radarr.added) != before+1 {
		t.Fatalf("missing: %q %v", done, err)
	}
	// A whole-show request: monitoring for later seasons comes back too, and the show is searched.
	sr, _ := s.Create(ctx, admin, New{Type: "series", TmdbID: 1396, Title: "Breaking Bad"})
	sonarr.cmds = nil
	if _, done, err = s.Fix(ctx, sr); err != nil || !strings.Contains(done, "Started a search") || len(sonarr.cmds) != 1 || sonarr.cmds[0]["name"] != "SeriesSearch" {
		t.Fatalf("series: %q %v %v", done, err, sonarr.cmds)
	}
	// Waiting for approval: nothing to fix.
	st.SetSetting(KeyApproval, "true")
	if _, _, err := s.Fix(ctx, store.Request{Status: "pending"}); err == nil {
		t.Fatal("fixed a waiting request")
	}
}

func TestExpiredByMaintainerr(t *testing.T) {
	s, st, radarr, _, _, user := setup(t)
	ctx := context.Background()
	r, _ := s.Create(ctx, user, New{Type: "movie", TmdbID: 603, Title: "The Matrix"})
	r.Status = "available"
	st.UpdateRequest(r)
	// It was in a Maintainerr collection, then Maintainerr deleted it from Radarr.
	st.SyncLeaving([]store.Leaving{{Type: "movie", TmdbID: 603, PlexID: "9", CollectionID: 3, Collection: "Movies leaving", LeaveAt: "2026-10-20T00:00:00Z"}}, "2026-10-13T00:00:00Z")
	radarr.gone = true
	s.Recheck(ctx, []string{"movie:603"}, false)
	if r, _ = st.Request(r.ID); r.Status != "expired" || !strings.Contains(r.Error, "Maintainer") {
		t.Fatalf("got %+v", r)
	}
	if a, _ := st.Bell(user.ID, store.CanNone); len(a) == 0 || a[0].Title != "Left Plex" {
		t.Fatalf("alert: %+v", a)
	}
	// Expired doesn't count against limits and can be asked for again.
	radarr.gone = false
	if _, err := s.Create(ctx, user, New{Type: "movie", TmdbID: 603, Title: "The Matrix"}); err != nil {
		t.Fatalf("request again: %v", err)
	}
}

func TestPlainWordsForUsers(t *testing.T) {
	removed := store.Request{Status: "removed", Error: "Removed from Plex and Sonarr."}
	if got := ForViewer(removed, false).Error; strings.Contains(got, "Sonarr") || !strings.Contains(got, "Removed from Plex") {
		t.Fatalf("removed: %q", got)
	}
	if got := ForViewer(removed, true); got.Error != ForViewer(removed, false).Error || got.Detail != removed.Error {
		t.Fatalf("admins: same words plus the detail: %+v", got)
	}
	failed := store.Request{Status: "failed", Error: "Radarr didn't add it: the API key was refused"}
	if got := PlainError(failed); strings.Contains(got, "Radarr") || strings.Contains(got, "API") {
		t.Fatalf("failed: %q", got)
	}
	if got := PlainError(store.Request{Status: "expired", Error: "Removed by Maintainerr (Movies leaving)."}); strings.Contains(got, "Maintainerr") {
		t.Fatalf("expired: %q", got)
	}
	if got := PlainError(store.Request{Status: "declined", Error: "Not out in good quality yet"}); got != "Not out in good quality yet" {
		t.Fatalf("declined keeps the admin's words: %q", got)
	}
	for _, l := range []Live{{Error: "couldn't reach it"}, {InArr: false}, {InArr: true, Monitored: false}} {
		if h := hintFor(store.Request{Type: "movie", Status: "sent"}, l); strings.Contains(h, "Radarr") || strings.Contains(h, "Fix") {
			t.Fatalf("hint for a user: %q", h)
		}
	}
}

func TestRemoveFromArr(t *testing.T) {
	s, st, radarr, _, admin, _ := setup(t)
	ctx := context.Background()
	if in, err := s.RemoveFromArr(ctx, "movie", 603); err != nil || in {
		t.Fatalf("not in Radarr: %v %v", in, err)
	}
	radarr.inLib = true
	r, _ := st.CreateRequest(store.Request{Type: "movie", TmdbID: 603, Title: "The Matrix", UserID: admin.ID, Status: "available", ArrID: 77})
	if in, err := s.RemoveFromArr(ctx, "movie", 603); err != nil || !in {
		t.Fatalf("remove: %v %v", in, err)
	}
	if len(radarr.deleted) != 1 || radarr.deleted[0] != "movie/77?deleteFiles=true&addImportExclusion=false" {
		t.Fatalf("asked Radarr: %v", radarr.deleted)
	}
	got, _ := st.Request(r.ID)
	if got.Status != "removed" || !strings.HasPrefix(got.Error, "Removed from Radarr") {
		t.Fatalf("request after: %+v", got)
	}
}

func TestFixTitleNobodyRequested(t *testing.T) {
	s, _, radarr, _, _, _ := setup(t)
	ctx := context.Background()
	if _, err := s.FixTitle(ctx, "movie", 603); err == nil || !strings.Contains(err.Error(), "isn't in Radarr") {
		t.Fatalf("not in Radarr: %v", err)
	}
	radarr.inLib = true
	done, err := s.FixTitle(ctx, "movie", 603)
	if err != nil || radarr.put["monitored"] != true || len(radarr.cmds) == 0 {
		t.Fatalf("fix: %q %v put %v cmds %v", done, err, radarr.put, radarr.cmds)
	}
}

func TestLiveForHidesReleaseNamesFromUsers(t *testing.T) {
	s, _, radarr, _, _, _ := setup(t)
	ctx := context.Background()
	radarr.inLib = true
	radarr.queue = `[{"size":1000,"sizeleft":250,"status":"downloading","title":"The.Matrix.1999.2160p"}]`
	user := s.LiveFor(ctx, "movie", 603, 77, nil, false)
	admin := s.LiveFor(ctx, "movie", 603, 77, nil, true)
	if !user.InArr || !user.HasFile || user.File == nil || user.File.Release != "" || user.Link != "" || len(user.Queue) != 1 || user.Queue[0].Release != "" || user.Queue[0].Percent != 75 {
		t.Fatalf("for users: %+v", user)
	}
	if admin.File.Release != "The.Matrix.1999.1080p" || admin.Queue[0].Release != "The.Matrix.1999.2160p" || !strings.HasSuffix(admin.Link, "/movie/the-matrix-1999") {
		t.Fatalf("for admins: %+v %+v", admin, admin.File)
	}
}

func TestFixEpisodes(t *testing.T) {
	s, st, _, sonarr, admin, _ := setup(t)
	ctx := context.Background()
	if _, err := s.FixEpisodes(ctx, 1396, [][2]int{{1, 1}}, For{UserID: admin.ID, Title: "Breaking Bad"}); err == nil {
		t.Fatal("fixed a show Sonarr doesn't have")
	}
	sonarr.inLib, sonarr.epFile = true, true
	eps, err := s.Episodes(ctx, 1396, map[int][]int{1: {1}})
	if err != nil || len(eps) != 3 || !eps[0].HasFile || !eps[0].InPlex || eps[1].HasFile {
		t.Fatalf("episodes: %+v %v", eps, err)
	}
	done, err := s.FixEpisodes(ctx, 1396, [][2]int{{1, 1}, {1, 2}}, For{UserID: admin.ID, Title: "Breaking Bad"})
	if err != nil || !strings.Contains(done, "Deleted 1 file") || !strings.Contains(done, "1 missing episode") {
		t.Fatalf("fix: %q %v", done, err)
	}
	if len(sonarr.deleted) != 1 || !strings.Contains(sonarr.deleted[0], "9001") {
		t.Fatalf("deleted: %v", sonarr.deleted)
	}
	if ids, _ := sonarr.monitor["episodeIds"].([]any); len(ids) != 2 || sonarr.monitor["monitored"] != true {
		t.Fatalf("monitored: %v", sonarr.monitor)
	}
	last := sonarr.cmds[len(sonarr.cmds)-1]
	if last["name"] != "EpisodeSearch" || len(last["episodeIds"].([]any)) != 2 {
		t.Fatalf("search: %v", last)
	}
	if list, _ := st.OpenRequests("series", 1396); len(list) != 1 || list[0].Source != "redownload" || list[0].Mode != "episodes" {
		t.Fatalf("its own request: %+v", list)
	}
}

func TestSeriesLookupIgnoresOtherShows(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"title":"Wings","tmdbId":2506,"tvdbId":71962,"id":394}]`)) // Sonarr's TVDB fallback
	}))
	defer srv.Close()
	got, err := seriesLookup(context.Background(), &arr.Client{URL: srv.URL, APIKey: "k"}, 71962)
	if err != nil || len(got) != 0 {
		t.Fatalf("matched another show: %v %v", got, err)
	}
}
