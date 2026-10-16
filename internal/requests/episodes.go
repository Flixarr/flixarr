package requests

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/Flixarr/flixarr/internal/arr"
	"github.com/Flixarr/flixarr/internal/store"
)

// Fixing single episodes (the Fix episodes window): missing ones are searched for again, ones with a file
// are redownloaded (the file deleted and the release it came from blocklisted, like Redownload).

// EpisodeState is one episode as the Fix episodes window shows it.
type EpisodeState struct {
	Season  int    `json:"season"`
	Episode int    `json:"episode"`
	Title   string `json:"title"`
	Aired   bool   `json:"aired"`
	HasFile bool   `json:"hasFile"`
	InPlex  bool   `json:"inPlex"`
}

// Episodes lists a series' episodes in Sonarr (specials left out), with what's in Plex.
func (s *Service) Episodes(ctx context.Context, tmdbID int, inPlex map[int][]int) ([]EpisodeState, error) {
	c, sr, err := s.sonarrSeries(ctx, tmdbID)
	if err != nil {
		return nil, err
	}
	eps, err := episodeList(ctx, c, num(sr["id"]), false)
	if err != nil {
		return nil, err
	}
	out := []EpisodeState{}
	for _, e := range eps {
		n, ep := num(e["seasonNumber"]), num(e["episodeNumber"])
		if n == 0 {
			continue
		}
		air, _ := e["airDateUtc"].(string)
		title, _ := e["title"].(string)
		has, _ := e["hasFile"].(bool)
		out = append(out, EpisodeState{Season: n, Episode: ep, Title: title, Aired: air != "" && air < nowUTC(), HasFile: has, InPlex: slices.Contains(inPlex[n], ep)})
	}
	sort.Slice(out, func(a, b int) bool {
		if out[a].Season != out[b].Season {
			return out[a].Season < out[b].Season
		}
		return out[a].Episode < out[b].Episode
	})
	return out, nil
}

func (s *Service) sonarrSeries(ctx context.Context, tmdbID int) (*arr.Client, map[string]any, error) {
	v, ok, err := s.Store.FirstServer("sonarr")
	if err != nil {
		return nil, nil, err
	}
	if !ok {
		return nil, nil, errors.New("Sonarr isn't connected")
	}
	c := client(v)
	sr, err := inArr(ctx, c, "series", tmdbID)
	if err != nil {
		return nil, nil, err
	}
	if sr == nil {
		return nil, nil, errors.New("it isn't in Sonarr")
	}
	return c, sr, nil
}

// FixEpisodes fixes the picked episodes ([season, episode]): with a file, the file is deleted and its
// release blocklisted; every picked episode is monitored and searched for. Requests for those seasons
// follow it again, and a title nobody requested gets its own request (like Redownload).
func (s *Service) FixEpisodes(ctx context.Context, tmdbID int, picked [][2]int, by For) (string, error) {
	if len(picked) == 0 {
		return "", errors.New("pick at least one episode")
	}
	c, sr, err := s.sonarrSeries(ctx, tmdbID)
	if err != nil {
		return "", err
	}
	id := num(sr["id"])
	eps, err := episodeList(ctx, c, id, false)
	if err != nil {
		return "", err
	}
	want := map[[2]int]bool{}
	for _, p := range picked {
		want[p] = true
	}
	ids, files, withFile, seasons := []int{}, []int{}, map[int]bool{}, []int{}
	for _, e := range eps {
		k := [2]int{num(e["seasonNumber"]), num(e["episodeNumber"])}
		if !want[k] {
			continue
		}
		ids = append(ids, num(e["id"]))
		if !slices.Contains(seasons, k[0]) {
			seasons = append(seasons, k[0])
		}
		if f := num(e["episodeFileId"]); f > 0 {
			withFile[num(e["id"])] = true
			if !slices.Contains(files, f) {
				files = append(files, f)
			}
		}
	}
	if len(ids) == 0 {
		return "", errors.New("Sonarr doesn't have those episodes")
	}
	notes := []string{}
	marked := 0
	if len(files) > 0 {
		var hist []map[string]any
		c.Get(ctx, fmt.Sprintf("history/series?seriesId=%d", id), &hist) // read before the delete adds to it
		if err := c.Delete(ctx, "episodefile/bulk", map[string]any{"episodeFileIds": files}); err != nil {
			return "", fmt.Errorf("Sonarr didn't delete the files: %w", err)
		}
		marked = markFailed(ctx, c, hist, func(h map[string]any) bool { return withFile[num(h["episodeId"])] })
		notes = append(notes, fmt.Sprintf("Deleted %d file%s.", len(files), plural(len(files))))
		if marked == 1 {
			notes = append(notes, "The release it came from won't be grabbed again.")
		} else if marked > 1 {
			notes = append(notes, "The releases they came from won't be grabbed again.")
		}
	}
	// Sonarr only grabs monitored episodes of a monitored series.
	if sr["monitored"] != true {
		var full map[string]any
		if err := c.Get(ctx, fmt.Sprintf("series/%d", id), &full); err == nil {
			full["monitored"] = true
			c.Put(ctx, fmt.Sprintf("series/%d", id), full, nil)
		}
	}
	if err := c.Put(ctx, "episode/monitor", map[string]any{"episodeIds": ids, "monitored": true}, nil); err != nil {
		return "", fmt.Errorf("Sonarr didn't turn the episodes on: %w", err)
	}
	search := ids
	if marked > 0 && autoRedownload(ctx, c) {
		search = slices.DeleteFunc(slices.Clone(ids), func(e int) bool { return withFile[e] }) // Sonarr searches for those itself
	}
	if len(search) > 0 {
		if err := c.Post(ctx, "command", map[string]any{"name": "EpisodeSearch", "episodeIds": search}, nil); err != nil {
			return "", fmt.Errorf("Sonarr didn't start a search: %w", err)
		}
	}
	if missing := len(ids) - len(withFile); missing > 0 {
		notes = append(notes, fmt.Sprintf("Searching for %d missing episode%s.", missing, plural(missing)))
	}
	slices.Sort(seasons)
	if s.followAgain(ctx, "series", tmdbID, seasons) == 0 {
		v, _, _ := s.Store.FirstServer("sonarr")
		r, err := s.Store.CreateRequest(store.Request{Type: "series", TmdbID: tmdbID, Title: by.Title, Year: by.Year, Poster: by.Poster, UserID: by.UserID,
			Status: "sent", ServerID: v.ID, ArrID: id, ProfileID: v.QualityProfileID, ProfileName: v.QualityProfileName, Mode: "episodes", Episodes: picked, Source: "redownload"})
		if err == nil {
			s.Publish(r)
		}
	}
	if s.Plex != nil && s.Plex.Connected() && len(files) > 0 {
		s.Plex.Rescan(ctx, "series")
	}
	s.Log.Info("episodes fixed", "tmdb", tmdbID, "episodes", picked)
	return strings.Join(append(notes, "Sonarr is looking for new copies."), " "), nil
}

func nowUTC() string { return time.Now().UTC().Format(time.RFC3339) }

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
