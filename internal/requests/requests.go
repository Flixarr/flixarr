// Package requests sends movie and series requests to Radarr and Sonarr and follows them until
// they're available.
package requests

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"github.com/Flixarr/flixarr/internal/arr"
	"github.com/Flixarr/flixarr/internal/store"
)

// Settings keys.
const KeyApproval = "requests_approval" // "true" = requests from non-admins wait for an admin

// Service creates, sends and follows requests.
type Service struct {
	Store *store.Store
	Log   *slog.Logger
}

// ErrAlready is returned when a title already has an open request.
var ErrAlready = errors.New("already requested")

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
	mode, seasons := "all", []int{}
	if in.Type == "series" && len(in.Seasons) > 0 {
		mode, seasons = "seasons", in.Seasons
	}
	status := "approved"
	if s.ApprovalRequired() && u.Role != "Admin" {
		status = "pending"
	}
	r, err := s.Store.CreateRequest(store.Request{Type: in.Type, TmdbID: in.TmdbID, Title: in.Title, Year: in.Year, Poster: in.Poster, Backdrop: in.Backdrop,
		UserID: u.ID, Status: status, ServerID: v.ID, ProfileID: profileID, ProfileName: profileName, Mode: mode, Seasons: seasons, Anime: in.Type == "series" && isAnime(in.Genres, in.Language)})
	if err != nil {
		return store.Request{}, err
	}
	if status == "approved" {
		r = s.Send(ctx, r)
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
			r.Status = "available"
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
	if id := num(series["id"]); id > 0 { // already in Sonarr
		r.ArrID, r.Status = id, "sent"
		return nil
	}
	want := map[int]bool{}
	for _, n := range r.Seasons {
		want[n] = true
	}
	if seasons, ok := series["seasons"].([]any); ok {
		for _, x := range seasons {
			if m, ok := x.(map[string]any); ok {
				n := num(m["seasonNumber"])
				m["monitored"] = n > 0 && (r.Mode == "all" || want[n])
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
	var added map[string]any
	if err := c.Post(ctx, "series", series, &added); err != nil {
		return fmt.Errorf("Sonarr didn't add it: %w", err)
	}
	r.ArrID, r.Status = num(added["id"]), "sent"
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

// Refresh moves sent requests along: downloading (with progress) while in the queue, available once
// the files are there. It runs with the every-minute Radarr/Sonarr status job.
func (s *Service) Refresh(ctx context.Context) error {
	reqs, err := s.Store.ActiveRequests()
	if err != nil || len(reqs) == 0 {
		return err
	}
	servers := map[string]store.Server{}
	for _, k := range []string{"radarr", "sonarr"} {
		if v, ok, _ := s.Store.FirstServer(k); ok {
			servers[k] = v
		}
	}
	for _, r := range reqs {
		v, ok := servers[kindOf(r.Type)]
		if !ok {
			continue
		}
		before := r
		if err := s.follow(ctx, v, &r); err != nil {
			continue // try again next minute
		}
		if r.Status != before.Status || r.Progress != before.Progress {
			s.Store.UpdateRequest(r)
		}
	}
	return nil
}

func (s *Service) follow(ctx context.Context, v store.Server, r *store.Request) error {
	c := client(v)
	if r.Type == "movie" {
		var m map[string]any
		if err := c.Get(ctx, fmt.Sprintf("movie/%d", r.ArrID), &m); err != nil {
			return err
		}
		if b, _ := m["hasFile"].(bool); b {
			r.Status, r.Progress = "available", 100
			return nil
		}
		return s.queueProgress(ctx, c, fmt.Sprintf("queue/details?movieId=%d", r.ArrID), r)
	}
	var sr map[string]any
	if err := c.Get(ctx, fmt.Sprintf("series/%d", r.ArrID), &sr); err != nil {
		return err
	}
	want := map[int]bool{}
	for _, n := range r.Seasons {
		want[n] = true
	}
	complete, counted := true, 0
	if seasons, ok := sr["seasons"].([]any); ok {
		for _, x := range seasons {
			m, _ := x.(map[string]any)
			n := num(m["seasonNumber"])
			if n == 0 || (r.Mode == "seasons" && !want[n]) {
				continue
			}
			st, _ := m["statistics"].(map[string]any)
			have, total := num(st["episodeFileCount"]), num(st["episodeCount"])
			if total == 0 {
				continue // nothing aired yet
			}
			counted++
			if have < total {
				complete = false
			}
		}
	}
	if counted > 0 && complete {
		r.Status, r.Progress = "available", 100
		return nil
	}
	return s.queueProgress(ctx, c, fmt.Sprintf("queue/details?seriesId=%d", r.ArrID), r)
}

// queueProgress marks a request downloading (with the average progress) while it has queue items.
func (s *Service) queueProgress(ctx context.Context, c *arr.Client, path string, r *store.Request) error {
	var items []map[string]any
	if err := c.Get(ctx, path, &items); err != nil {
		return err
	}
	if len(items) == 0 {
		if r.Status == "downloading" {
			r.Status = "sent" // finished downloading but not imported yet, or removed from the queue
		}
		r.Progress = 0
		return nil
	}
	total, left := 0.0, 0.0
	for _, it := range items {
		total += numf(it["size"])
		left += numf(it["sizeleft"])
	}
	r.Status = "downloading"
	if total > 0 {
		r.Progress = int((total - left) / total * 100)
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
