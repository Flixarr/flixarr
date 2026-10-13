package requests

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/Flixarr/flixarr/internal/arr"
	"github.com/Flixarr/flixarr/internal/store"
)

// Redownload replaces a bad file: the release it came from is marked failed in Radarr/Sonarr (so it's
// blocklisted and not grabbed again), the file is deleted, and a new copy is searched for. For a series
// it works per season (no seasons = every season with files). Requests for the title follow it again.
// Without download history (a file added by hand) it just deletes the file and searches.
func (s *Service) Redownload(ctx context.Context, typ string, tmdbID int, seasons []int) (string, error) {
	v, ok, err := s.Store.FirstServer(kindOf(typ))
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("%s isn't connected", arr.Kinds[kindOf(typ)])
	}
	c := client(v)
	var done string
	if typ == "movie" {
		done, err = s.redownloadMovie(ctx, c, tmdbID)
	} else {
		done, err = s.redownloadSeries(ctx, c, v, tmdbID, seasons)
	}
	if err != nil {
		return "", err
	}
	s.followAgain(ctx, typ, tmdbID, seasons)
	if s.Plex != nil && s.Plex.Connected() {
		s.Plex.Rescan(ctx, typ) // Plex notices the file is gone (and the title leaves the library)
	}
	s.Log.Info("redownload", "type", typ, "tmdb", tmdbID, "seasons", seasons, "done", done)
	return done, nil
}

func (s *Service) redownloadMovie(ctx context.Context, c *arr.Client, tmdbID int) (string, error) {
	var have []map[string]any
	if err := c.Get(ctx, fmt.Sprintf("movie?tmdbId=%d", tmdbID), &have); err != nil {
		return "", err
	}
	if len(have) == 0 {
		return "", errors.New("it isn't in Radarr")
	}
	id := num(have[0]["id"])
	file, _ := have[0]["movieFile"].(map[string]any)
	if b, _ := have[0]["hasFile"].(bool); !b || file == nil {
		return "", errors.New("Radarr has no file for it")
	}
	var hist []map[string]any
	c.Get(ctx, fmt.Sprintf("history/movie?movieId=%d", id), &hist) // read before the delete adds to it
	// Order matters: delete first (a search while the old file is there finds nothing better), monitor
	// it again (Radarr can unmonitor movies whose file was deleted), then mark the grab failed.
	if err := c.Delete(ctx, fmt.Sprintf("moviefile/%d", num(file["id"]))); err != nil {
		return "", fmt.Errorf("Radarr didn't delete the file: %w", err)
	}
	var movie map[string]any
	if err := c.Get(ctx, fmt.Sprintf("movie/%d", id), &movie); err == nil && movie["monitored"] != true {
		movie["monitored"] = true
		if err := c.Put(ctx, fmt.Sprintf("movie/%d", id), movie, nil); err != nil {
			return "", fmt.Errorf("Radarr didn't turn it back on: %w", err)
		}
	}
	marked := markFailed(ctx, c, hist, func(map[string]any) bool { return true })
	notes := []string{"Deleted the file."}
	if marked > 0 {
		notes = append(notes, "The release it came from won't be grabbed again.")
	}
	if marked == 0 || !autoRedownload(ctx, c) {
		if err := c.Post(ctx, "command", map[string]any{"name": "MoviesSearch", "movieIds": []int{id}}, nil); err != nil {
			return "", fmt.Errorf("Radarr didn't start a search: %w", err)
		}
	}
	return strings.Join(append(notes, "Radarr is looking for a new copy."), " "), nil
}

func (s *Service) redownloadSeries(ctx context.Context, c *arr.Client, v store.Server, tmdbID int, seasons []int) (string, error) {
	var found []map[string]any
	if err := c.Get(ctx, "series/lookup?term="+url.QueryEscape(fmt.Sprintf("tmdb:%d", tmdbID)), &found); err != nil {
		return "", err
	}
	if len(found) == 0 || num(found[0]["id"]) == 0 {
		return "", errors.New("it isn't in Sonarr")
	}
	id := num(found[0]["id"])
	eps, err := episodeList(ctx, c, id, false)
	if err != nil {
		return "", err
	}
	want := map[int]bool{}
	for _, n := range seasons {
		want[n] = true
	}
	inSeason := map[int]int{} // episode ID → season
	files, picked, hadFile := []int{}, []int{}, []int{}
	for _, e := range eps {
		n := num(e["seasonNumber"])
		if n == 0 || (len(want) > 0 && !want[n]) {
			continue
		}
		inSeason[num(e["id"])] = n
		if f := num(e["episodeFileId"]); f > 0 {
			hadFile = append(hadFile, num(e["id"]))
		}
		if f := num(e["episodeFileId"]); f > 0 && !slices.Contains(files, f) {
			files = append(files, f)
			if !slices.Contains(picked, n) {
				picked = append(picked, n)
			}
		}
	}
	if len(files) == 0 {
		return "", errors.New("Sonarr has no files for that")
	}
	var hist []map[string]any
	c.Get(ctx, fmt.Sprintf("history/series?seriesId=%d", id), &hist)
	// Delete, monitor again (Sonarr can unmonitor episodes whose files were deleted), then mark failed.
	if err := c.Delete(ctx, "episodefile/bulk", map[string]any{"episodeFileIds": files}); err != nil {
		return "", fmt.Errorf("Sonarr didn't delete the files: %w", err)
	}
	if err := c.Put(ctx, "episode/monitor", map[string]any{"episodeIds": hadFile, "monitored": true}, nil); err != nil {
		return "", fmt.Errorf("Sonarr didn't turn the episodes back on: %w", err)
	}
	slices.Sort(picked)
	set := map[int]bool{}
	for _, n := range picked {
		set[n] = true
	}
	s.monitorSeasons(ctx, c, store.Server{}, id, false, set)
	marked := markFailed(ctx, c, hist, func(h map[string]any) bool { _, ok := inSeason[num(h["episodeId"])]; return ok })
	notes := []string{fmt.Sprintf("Deleted %d file%s.", len(files), map[bool]string{true: "", false: "s"}[len(files) == 1])}
	if marked > 0 {
		notes = append(notes, "The releases they came from won't be grabbed again.")
	}
	if marked == 0 || !autoRedownload(ctx, c) {
		for _, n := range picked {
			c.Post(ctx, "command", map[string]any{"name": "SeasonSearch", "seriesId": id, "seasonNumber": n}, nil)
		}
	}
	return strings.Join(append(notes, "Sonarr is looking for new copies."), " "), nil
}

// markFailed marks as failed the grabs that the current files came from (newest import per download),
// which blocklists those releases. It returns how many were marked.
func markFailed(ctx context.Context, c *arr.Client, hist []map[string]any, keep func(map[string]any) bool) int {
	imported := map[string]bool{}
	for _, h := range hist { // newest first: the latest import of each download is what's on disk
		if t, _ := h["eventType"].(string); t == "downloadFolderImported" && keep(h) {
			if d, _ := h["downloadId"].(string); d != "" {
				imported[d] = true
			}
		}
	}
	marked := 0
	for _, h := range hist {
		d, _ := h["downloadId"].(string)
		if t, _ := h["eventType"].(string); t != "grabbed" || !imported[d] {
			continue
		}
		delete(imported, d) // one grab per download (a season pack has one per episode)
		if c.Post(ctx, fmt.Sprintf("history/failed/%d", num(h["id"])), nil, nil) == nil {
			marked++
		}
	}
	return marked
}

// autoRedownload: Radarr/Sonarr searches again on its own after a failed download (their default).
func autoRedownload(ctx context.Context, c *arr.Client) bool {
	var cfg map[string]any
	if c.Get(ctx, "config/downloadclient", &cfg) != nil {
		return false
	}
	on, _ := cfg["autoRedownloadFailed"].(bool)
	return on
}

// followAgain puts the title's finished requests (for those seasons) back to following, so they show
// the new download and turn Available again on their own.
func (s *Service) followAgain(ctx context.Context, typ string, tmdbID int, seasons []int) {
	list, err := s.Store.OpenRequests(typ, tmdbID)
	if err != nil {
		return
	}
	for _, r := range list {
		if r.Status != "available" || r.ArrID == 0 {
			continue
		}
		if typ == "series" && len(seasons) > 0 && r.Mode == "seasons" && !overlaps(r.Seasons, seasons) {
			continue
		}
		r.Status, r.Progress = "sent", 0
		s.Store.UpdateRequest(r)
		s.Publish(r)
	}
}

func overlaps(a, b []int) bool {
	for _, x := range a {
		if slices.Contains(b, x) {
			return true
		}
	}
	return false
}
