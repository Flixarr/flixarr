// Package requests sends movie and series requests to Radarr and Sonarr and follows them until
// they're available.
package requests

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Flixarr/flixarr/internal/arr"
	"github.com/Flixarr/flixarr/internal/store"
)

// Settings keys.
const KeyApproval = "requests_approval" // "true" = requests from non-admins wait for an admin

// Service creates, sends and follows requests.
type Service struct {
	Store    *store.Store
	Log      *slog.Logger
	Notify   func(event string, r store.Request) // Settings → Notifications; nil = none
	Plex     Plex                                // nil = no Plex checks (tests)
	OnChange func(before, after store.Request)   // a request's status or progress changed (bell alerts)

	mu         sync.Mutex
	lastRecent time.Time
	rescanned  map[int64]time.Time
}

func (s *Service) notify(event string, r store.Request) {
	if s.Notify != nil {
		s.Notify(event, r)
	}
}

// ErrAlready is returned when a title already has an open request.
var ErrAlready = errors.New("already requested")

// ErrNotAllowed is returned when the user may not request this type.
var ErrNotAllowed = errors.New("not allowed")

// ErrBlocked is returned for titles on the blocklist.
var ErrBlocked = errors.New("blocked")

// ErrInPlex is returned when the title is already in Plex.
var ErrInPlex = errors.New("already in Plex")

func inPlex(st *store.Store, typ string, tmdbID int) bool {
	got, _ := st.PlexItemsFor([]string{fmt.Sprintf("%s:%d", typ, tmdbID)})
	return len(got) > 0
}

// New is what someone asks for.
type New struct {
	Type      string   `json:"type"` // movie | series
	TmdbID    int      `json:"tmdbId"`
	Title     string   `json:"title"`
	Year      string   `json:"year"`
	Poster    string   `json:"poster"`
	Backdrop  string   `json:"backdrop"`
	Seasons   []int    `json:"seasons"`   // series: empty = the whole show
	ProfileID int      `json:"profileId"` // 0 = the server's default quality
	Genres    []string `json:"genres"`    // for anime detection
	Language  string   `json:"language"`  // original language, e.g. "ja"
	// SeasonCount is how many seasons the show has (series, whole-show requests), for request limits.
	SeasonCount int `json:"seasonCount"`
	// Episodes asks for single episodes ([season, episode] pairs) instead of whole seasons.
	Episodes [][2]int `json:"episodes"`
}

// KeyTVModes is Settings → Requests: which ways of requesting a series are offered, and the default.
const KeyTVModes = "tv_modes"

// TVModes is how series can be requested: the whole show, picked seasons, or picked episodes.
type TVModes struct {
	Whole    bool   `json:"whole"`
	Seasons  bool   `json:"seasons"`
	Episodes bool   `json:"episodes"`
	Default  string `json:"default"` // whole | seasons | episodes
}

// Modes returns the series request modes (all on, whole show first, until changed).
func (s *Service) Modes() TVModes {
	m := TVModes{Whole: true, Seasons: true, Episodes: true, Default: "whole"}
	json.Unmarshal([]byte(s.Store.Setting(KeyTVModes, "")), &m)
	return m
}

// SetModes saves the series request modes; at least one stays on, and the default is one that's on.
func (s *Service) SetModes(m TVModes) (TVModes, error) {
	if !m.Whole && !m.Seasons && !m.Episodes {
		return m, errors.New("keep at least one way to request series")
	}
	on := map[string]bool{"whole": m.Whole, "seasons": m.Seasons, "episodes": m.Episodes}
	if !on[m.Default] {
		for _, k := range []string{"whole", "seasons", "episodes"} {
			if on[k] {
				m.Default = k
				break
			}
		}
	}
	b, _ := json.Marshal(m)
	return m, s.Store.SetSetting(KeyTVModes, string(b))
}

// LimitError is returned when a request would go over the user's weekly limit.
type LimitError struct{ Msg string }

func (e LimitError) Error() string { return e.Msg }

// Usage says how much of a weekly limit is used (Limit 0 = unlimited).
type Usage struct {
	Limit int `json:"limit"`
	Used  int `json:"used"`
}

// UsageFor is the user's weekly movie or season usage (admins have no limits).
func (s *Service) UsageFor(u *store.User, typ string) Usage {
	if u.Can("admin") {
		return Usage{}
	}
	movies, seasons, _ := s.Store.RecentUsage(u.ID)
	if typ == "movie" {
		return Usage{Limit: u.MovieLimit, Used: movies}
	}
	return Usage{Limit: u.SeasonLimit, Used: seasons}
}

// ApprovalRequired reports whether requests from non-admins wait for an admin.
func (s *Service) ApprovalRequired() bool { return s.Store.Setting(KeyApproval, "false") == "true" }

func kindOf(typ string) string {
	if typ == "series" {
		return "sonarr"
	}
	return "radarr"
}

// Create saves a request and, unless it needs approval, sends it straight away.
func (s *Service) Create(ctx context.Context, u *store.User, in New) (store.Request, error) {
	if in.Type != "movie" && in.Type != "series" {
		return store.Request{}, errors.New("unknown type")
	}
	if in.TmdbID <= 0 || strings.TrimSpace(in.Title) == "" {
		return store.Request{}, errors.New("missing title")
	}
	if s.Store.IsBlocked(in.Type, in.TmdbID) {
		return store.Request{}, ErrBlocked
	}
	if (in.Type == "movie" && !u.Can("requestMovies")) || (in.Type == "series" && !u.Can("requestTv")) {
		return store.Request{}, ErrNotAllowed
	}
	if !u.Can("advanced") {
		in.ProfileID = 0 // only people allowed to choose get a quality other than the default
	}
	// A movie in Plex is done. A series can still be missing seasons, so it can be requested.
	if in.Type == "movie" && inPlex(s.Store, in.Type, in.TmdbID) {
		return store.Request{}, ErrInPlex
	}
	if _, open, err := s.Store.OpenRequest(in.Type, in.TmdbID); err != nil {
		return store.Request{}, err
	} else if open {
		return store.Request{}, ErrAlready
	}
	v, ok, err := s.Store.FirstServer(kindOf(in.Type))
	if err != nil {
		return store.Request{}, err
	}
	if !ok || !v.Enabled {
		return store.Request{}, fmt.Errorf("%s isn't connected yet (Settings → Services)", arr.Kinds[kindOf(in.Type)])
	}
	profileID, profileName := v.QualityProfileID, v.QualityProfileName
	if in.ProfileID != 0 && in.ProfileID != profileID {
		var list []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		}
		if err := client(v).Get(ctx, "qualityprofile", &list); err != nil {
			return store.Request{}, err
		}
		found := false
		for _, p := range list {
			if p.ID == in.ProfileID {
				profileID, profileName, found = p.ID, p.Name, true
			}
		}
		if !found {
			return store.Request{}, errors.New("that quality isn't on the server anymore")
		}
	}
	mode, seasons, episodes := "all", []int{}, [][2]int{}
	if in.Type == "series" && len(in.Episodes) > 0 {
		mode, episodes = "episodes", in.Episodes
	} else if in.Type == "series" && len(in.Seasons) > 0 {
		mode, seasons = "seasons", in.Seasons
	}
	if in.Type == "series" {
		modes := s.Modes()
		if (mode == "all" && !modes.Whole) || (mode == "seasons" && !modes.Seasons) || (mode == "episodes" && !modes.Episodes) {
			return store.Request{}, errors.New("that way of requesting series is turned off")
		}
	}
	count := 0
	if in.Type == "series" {
		count = max(in.SeasonCount, 1)
		switch mode {
		case "seasons":
			count = len(seasons)
		case "episodes": // counts the seasons the episodes are in
			in := map[int]bool{}
			for _, e := range episodes {
				in[e[0]] = true
			}
			count = len(in)
		}
	}
	if use := s.UsageFor(u, in.Type); use.Limit > 0 {
		want := 1
		what := "movie"
		if in.Type == "series" {
			want, what = count, "season"
		}
		if use.Used+want > use.Limit {
			left := max(use.Limit-use.Used, 0)
			return store.Request{}, LimitError{fmt.Sprintf("that's over your limit of %d %ss a week (%d left)", use.Limit, what, left)}
		}
	}
	status := "approved"
	if s.ApprovalRequired() && !u.Can("autoApprove") {
		status = "pending"
	}
	r, err := s.Store.CreateRequest(store.Request{Type: in.Type, TmdbID: in.TmdbID, Title: in.Title, Year: in.Year, Poster: in.Poster, Backdrop: in.Backdrop,
		UserID: u.ID, Status: status, ServerID: v.ID, ProfileID: profileID, ProfileName: profileName, Mode: mode, Seasons: seasons, Episodes: episodes, Anime: in.Type == "series" && isAnime(in.Genres, in.Language), SeasonCount: count})
	if err != nil {
		return store.Request{}, err
	}
	if status == "approved" {
		r = s.Send(ctx, r)
	}
	s.notify("request_submitted", r)
	if r.Status == "pending" || r.Status == "failed" {
		s.Alert(r)
	}
	return r, nil
}

func client(v store.Server) *arr.Client {
	return &arr.Client{Kind: v.Kind, URL: v.URL, APIKey: v.APIKey}
}

// Send adds an approved request's title to Radarr/Sonarr. A title already there is linked instead of
// added again. The returned request has its new status (sent, available or failed with the reason).
func (s *Service) Send(ctx context.Context, r store.Request) store.Request {
	v, ok, err := s.Store.FirstServer(kindOf(r.Type))
	if err == nil && !ok {
		err = fmt.Errorf("%s isn't connected", arr.Kinds[kindOf(r.Type)])
	}
	if err == nil {
		r.ServerID = v.ID
		if r.ProfileID == 0 {
			r.ProfileID, r.ProfileName = v.QualityProfileID, v.QualityProfileName
		}
		if r.Type == "movie" {
			err = s.sendMovie(ctx, v, &r)
		} else {
			err = s.sendSeries(ctx, v, &r)
		}
	}
	if err != nil {
		r.Status, r.Error = "failed", capitalize(err.Error())
		s.Log.Warn("request failed", "title", r.Title, "err", err)
		defer s.notify("request_failed", r)
	} else {
		r.Error = ""
		s.Log.Info("request sent", "title", r.Title, "type", r.Type, "status", r.Status)
	}
	s.Store.UpdateRequest(r)
	return r
}

func (s *Service) sendMovie(ctx context.Context, v store.Server, r *store.Request) error {
	c := client(v)
	// Already in Radarr? (Radarr 6's lookup doesn't say, so ask the library directly.)
	var have []map[string]any
	if err := c.Get(ctx, fmt.Sprintf("movie?tmdbId=%d", r.TmdbID), &have); err == nil && len(have) > 0 {
		r.ArrID, r.Status = num(have[0]["id"]), "sent"
		if b, _ := have[0]["hasFile"].(bool); b {
			s.filesDone(ctx, r, nil)
		}
		return nil
	}
	var movie map[string]any
	if err := c.Get(ctx, fmt.Sprintf("movie/lookup/tmdb?tmdbId=%d", r.TmdbID), &movie); err != nil {
		return fmt.Errorf("Radarr couldn't find it: %w", err)
	}
	movie["qualityProfileId"] = r.ProfileID
	movie["rootFolderPath"] = v.RootFolder
	movie["monitored"] = true
	movie["minimumAvailability"] = "released"
	movie["addOptions"] = map[string]any{"searchForMovie": v.SearchOnAdd}
	var added map[string]any
	if err := c.Post(ctx, "movie", movie, &added); err != nil {
		return fmt.Errorf("Radarr didn't add it: %w", err)
	}
	r.ArrID, r.Status = num(added["id"]), "sent"
	return nil
}

func (s *Service) sendSeries(ctx context.Context, v store.Server, r *store.Request) error {
	c := client(v)
	var found []map[string]any
	if err := c.Get(ctx, "series/lookup?term="+url.QueryEscape(fmt.Sprintf("tmdb:%d", r.TmdbID)), &found); err != nil {
		return fmt.Errorf("Sonarr couldn't find it: %w", err)
	}
	if len(found) == 0 {
		return errors.New("Sonarr couldn't find this series")
	}
	series := found[0]
	want := map[int]bool{}
	for _, n := range r.Seasons {
		want[n] = true
	}
	if id := num(series["id"]); id > 0 { // already in Sonarr: monitor the requested seasons (or episodes) and search
		r.ArrID, r.Status = id, "sent"
		if r.Mode == "episodes" {
			return s.monitorEpisodes(ctx, c, v, id, r.Episodes)
		}
		return s.monitorSeasons(ctx, c, v, id, r.Mode == "all", want)
	}
	if seasons, ok := series["seasons"].([]any); ok {
		for _, x := range seasons {
			if m, ok := x.(map[string]any); ok {
				n := num(m["seasonNumber"])
				m["monitored"] = n > 0 && (r.Mode == "all" || want[n]) // episodes mode: none; the episodes are picked after
			}
		}
	}
	series["qualityProfileId"] = r.ProfileID
	series["rootFolderPath"] = v.RootFolder
	series["seasonFolder"] = v.SeasonFolders
	series["monitored"] = true
	series["seriesType"] = "standard"
	if r.Anime {
		series["seriesType"] = "anime"
	}
	series["addOptions"] = map[string]any{"ignoreEpisodesWithFiles": true, "searchForMissingEpisodes": v.SearchOnAdd}
	if r.Mode == "episodes" {
		series["addOptions"] = map[string]any{"monitor": "none", "searchForMissingEpisodes": false}
	}
	var added map[string]any
	if err := c.Post(ctx, "series", series, &added); err != nil {
		return fmt.Errorf("Sonarr didn't add it: %w", err)
	}
	if r.Mode == "episodes" {
		r.ArrID, r.Status = num(added["id"]), "sent"
		return s.monitorEpisodes(ctx, c, v, r.ArrID, r.Episodes)
	}
	r.ArrID, r.Status = num(added["id"]), "sent"
	return nil
}

// monitorSeasons turns on the requested seasons of a series already in Sonarr (it never turns any off)
// and, if the server searches on request, searches for the newly monitored ones.
func (s *Service) monitorSeasons(ctx context.Context, c *arr.Client, v store.Server, id int, all bool, want map[int]bool) error {
	var sr map[string]any
	if err := c.Get(ctx, fmt.Sprintf("series/%d", id), &sr); err != nil {
		return fmt.Errorf("Sonarr couldn't read the series: %w", err)
	}
	var added []int
	if seasons, ok := sr["seasons"].([]any); ok {
		for _, x := range seasons {
			m, _ := x.(map[string]any)
			n := num(m["seasonNumber"])
			if on, _ := m["monitored"].(bool); n > 0 && !on && (all || want[n]) {
				m["monitored"] = true
				added = append(added, n)
			}
		}
	}
	if len(added) == 0 && sr["monitored"] == true {
		return nil // everything asked for is already monitored
	}
	sr["monitored"] = true
	if err := c.Put(ctx, fmt.Sprintf("series/%d", id), sr, nil); err != nil {
		return fmt.Errorf("Sonarr didn't update the series: %w", err)
	}
	if v.SearchOnAdd {
		for _, n := range added {
			c.Post(ctx, "command", map[string]any{"name": "SeasonSearch", "seriesId": id, "seasonNumber": n}, nil)
		}
	}
	return nil
}

// episodeList reads a series' episodes from Sonarr. Right after adding a series Sonarr is still filling
// them in, so it waits a little for them.
func episodeList(ctx context.Context, c *arr.Client, seriesID int, wait bool) ([]map[string]any, error) {
	for try := 0; ; try++ {
		var eps []map[string]any
		if err := c.Get(ctx, fmt.Sprintf("episode?seriesId=%d", seriesID), &eps); err != nil {
			return nil, err
		}
		if len(eps) > 0 || !wait || try >= 10 {
			return eps, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(1500 * time.Millisecond):
		}
	}
}

// monitorEpisodes turns on the requested episodes of a series in Sonarr and, if the server searches on
// request, searches for them.
func (s *Service) monitorEpisodes(ctx context.Context, c *arr.Client, v store.Server, seriesID int, want [][2]int) error {
	eps, err := episodeList(ctx, c, seriesID, true)
	if err != nil {
		return fmt.Errorf("Sonarr couldn't list the episodes: %w", err)
	}
	asked := map[[2]int]bool{}
	for _, e := range want {
		asked[e] = true
	}
	ids := []int{}
	for _, e := range eps {
		if asked[[2]int{num(e["seasonNumber"]), num(e["episodeNumber"])}] {
			ids = append(ids, num(e["id"]))
		}
	}
	if len(ids) == 0 {
		return errors.New("Sonarr doesn't have those episodes listed yet")
	}
	if err := c.Put(ctx, "episode/monitor", map[string]any{"episodeIds": ids, "monitored": true}, nil); err != nil {
		return fmt.Errorf("Sonarr didn't turn the episodes on: %w", err)
	}
	if v.SearchOnAdd {
		c.Post(ctx, "command", map[string]any{"name": "EpisodeSearch", "episodeIds": ids}, nil)
	}
	return nil
}

// isAnime: Japanese animation is sent as Sonarr's anime type (its episodes are numbered differently).
func isAnime(genres []string, language string) bool {
	if language != "ja" {
		return false
	}
	for _, g := range genres {
		if strings.EqualFold(g, "Animation") {
			return true
		}
	}
	return false
}

// Plex is what requests need from the Plex connection: whether it's set up, what's in it, and a way
// to make it look for new files (plexlib.Library).
type Plex interface {
	Connected() bool
	Seasons(ctx context.Context, tmdbID int) (map[int]int, error)
	Rescan(ctx context.Context, typ string) error // ask Plex to scan its movie or TV libraries
	RecentScan(ctx context.Context) (string, error)
}

// Refresh moves active requests along: grabbed → downloading (with progress) → importing → available.
// Available means it's in Plex when a Plex server is connected (otherwise: Radarr/Sonarr has the files).
// The request follow loop runs it every few seconds while anything is active, and webhooks run it at once.
func (s *Service) Refresh(ctx context.Context) error {
	reqs, err := s.Store.ActiveRequests()
	if err != nil || len(reqs) == 0 {
		return err
	}
	return s.refresh(ctx, reqs)
}

// Nudge follows the requests for one Radarr/Sonarr item right away (a webhook arrived for it).
func (s *Service) Nudge(ctx context.Context, typ string, arrID int) error {
	reqs, err := s.Store.ActiveRequestsFor(typ, arrID)
	if err != nil || len(reqs) == 0 {
		return err
	}
	return s.refresh(ctx, reqs)
}

func (s *Service) refresh(ctx context.Context, reqs []store.Request) error {
	s.mu.Lock() // the loop and webhooks can overlap
	defer s.mu.Unlock()
	servers := map[string]store.Server{}
	for _, k := range []string{"radarr", "sonarr"} {
		if v, ok, _ := s.Store.FirstServer(k); ok {
			servers[k] = v
		}
	}
	// Something waiting for Plex: read Plex's newest additions first (cheap), so it's seen within seconds.
	for _, r := range reqs {
		if r.Status == "importing" && s.Plex != nil && s.Plex.Connected() && time.Since(s.lastRecent) > 10*time.Second {
			s.lastRecent = time.Now()
			s.Plex.RecentScan(ctx)
			break
		}
	}
	for _, r := range reqs {
		v, ok := servers[kindOf(r.Type)]
		if !ok {
			continue
		}
		before := r
		if r.Type == "movie" && inPlex(s.Store, r.Type, r.TmdbID) {
			r.Status, r.Progress = "available", 100 // Plex has it, however it got there
		} else if err := s.follow(ctx, v, &r); err != nil {
			continue // try again on the next pass
		}
		if r.Status != before.Status {
			s.Alert(r)
		}
		if r.Status != before.Status || r.Progress != before.Progress {
			s.Store.UpdateRequest(r)
			if s.OnChange != nil {
				s.OnChange(before, r)
			}
		}
		if r.Status == "available" && before.Status != "available" {
			s.notify("request_available", r)
		}
	}
	return nil
}

// plexHas says whether Plex has what a request asked for. ok=false when no Plex server is connected.
func (s *Service) plexHas(ctx context.Context, r *store.Request, sonarrHave map[int]int) (has, ok bool) {
	if s.Plex == nil || !s.Plex.Connected() {
		return false, false
	}
	if r.Type == "movie" {
		return inPlex(s.Store, "movie", r.TmdbID), true
	}
	counts, err := s.Plex.Seasons(ctx, r.TmdbID)
	if err != nil {
		return false, true
	}
	for n, have := range sonarrHave {
		if counts[n] < have {
			return false, true
		}
	}
	return len(sonarrHave) > 0, true
}

// filesDone: Radarr/Sonarr has everything asked for. Then it's importing until Plex has it too.
func (s *Service) filesDone(ctx context.Context, r *store.Request, sonarrHave map[int]int) {
	has, ok := s.plexHas(ctx, r, sonarrHave)
	if !ok || has {
		r.Status, r.Progress = "available", 100
		return
	}
	if r.Status != "importing" || time.Since(s.rescanned[r.ID]) > 2*time.Minute {
		// Plex didn't notice the new files on its own (yet): ask it to scan, at most every 2 minutes.
		if s.rescanned == nil {
			s.rescanned = map[int64]time.Time{}
		}
		s.rescanned[r.ID] = time.Now()
		if err := s.Plex.Rescan(ctx, r.Type); err != nil {
			s.Log.Warn("plex rescan", "err", err)
		}
	}
	r.Status, r.Progress = "importing", 100
}

func (s *Service) follow(ctx context.Context, v store.Server, r *store.Request) error {
	c := client(v)
	if r.Type == "movie" {
		var m map[string]any
		if err := c.Get(ctx, fmt.Sprintf("movie/%d", r.ArrID), &m); err != nil {
			return err
		}
		if b, _ := m["hasFile"].(bool); b {
			s.filesDone(ctx, r, nil)
			return nil
		}
		return s.queueProgress(ctx, c, fmt.Sprintf("queue/details?movieId=%d", r.ArrID), r)
	}
	if r.Mode == "episodes" {
		eps, err := episodeList(ctx, c, r.ArrID, false)
		if err != nil {
			return err
		}
		asked := map[[2]int]bool{}
		for _, e := range r.Episodes {
			asked[e] = true
		}
		have, done, seen := map[int]int{}, true, 0
		for _, e := range eps {
			k := [2]int{num(e["seasonNumber"]), num(e["episodeNumber"])}
			if !asked[k] {
				continue
			}
			seen++
			if f, _ := e["hasFile"].(bool); f {
				have[k[0]]++
			} else {
				done = false
			}
		}
		if seen > 0 && done {
			s.filesDone(ctx, r, have)
			return nil
		}
		return s.queueProgress(ctx, c, fmt.Sprintf("queue/details?seriesId=%d", r.ArrID), r)
	}
	var sr map[string]any
	if err := c.Get(ctx, fmt.Sprintf("series/%d", r.ArrID), &sr); err != nil {
		return err
	}
	want := map[int]bool{}
	for _, n := range r.Seasons {
		want[n] = true
	}
	complete, have := true, map[int]int{}
	if seasons, ok := sr["seasons"].([]any); ok {
		for _, x := range seasons {
			m, _ := x.(map[string]any)
			n := num(m["seasonNumber"])
			if n == 0 || (r.Mode == "seasons" && !want[n]) {
				continue
			}
			st, _ := m["statistics"].(map[string]any)
			files, total := num(st["episodeFileCount"]), num(st["episodeCount"])
			if total == 0 {
				continue // nothing aired yet
			}
			have[n] = files
			if files < total {
				complete = false
			}
		}
	}
	if len(have) > 0 && complete {
		s.filesDone(ctx, r, have)
		return nil
	}
	return s.queueProgress(ctx, c, fmt.Sprintf("queue/details?seriesId=%d", r.ArrID), r)
}

// queueProgress reads the download queue: grabbed (waiting in the download client), downloading with the
// average progress, or importing (downloaded, Radarr/Sonarr moving the files).
func (s *Service) queueProgress(ctx context.Context, c *arr.Client, path string, r *store.Request) error {
	var items []map[string]any
	if err := c.Get(ctx, path, &items); err != nil {
		return err
	}
	if len(items) == 0 {
		if r.Status == "grabbed" || r.Status == "downloading" || r.Status == "importing" {
			r.Status = "sent" // removed from the queue without files: searching again
		}
		r.Progress = 0
		return nil
	}
	total, left, importing, moving := 0.0, 0.0, false, false
	for _, it := range items {
		total += numf(it["size"])
		left += numf(it["sizeleft"])
		state, _ := it["trackedDownloadState"].(string)
		status, _ := it["status"].(string)
		if state == "importPending" || state == "importing" || state == "imported" || status == "completed" {
			importing = true
		}
		if status == "downloading" {
			moving = true
		}
	}
	r.Progress = 0
	if total > 0 {
		r.Progress = int((total - left) / total * 100)
	}
	switch {
	case importing && !moving:
		r.Status, r.Progress = "importing", 100
	case moving || r.Progress > 0:
		r.Status = "downloading"
	default:
		r.Status = "grabbed" // queued, delayed or paused in the download client
	}
	return nil
}

func num(v any) int {
	f, _ := v.(float64)
	return int(f)
}

func numf(v any) float64 {
	f, _ := v.(float64)
	return f
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
