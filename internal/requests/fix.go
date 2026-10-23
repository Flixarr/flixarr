package requests

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Flixarr/flixarr/internal/arr"
	"github.com/Flixarr/flixarr/internal/store"
)

// Fix repairs a request that seems stuck: added again when it's missing from Radarr/Sonarr, monitoring
// turned back on for what was asked, and a search started either way. It returns the request and what
// was done, in plain words.
func (s *Service) Fix(ctx context.Context, r store.Request) (store.Request, string, error) {
	switch r.Status {
	case "pending", "declined":
		return r, "", errors.New("it hasn't been approved")
	case "available":
		return r, "", errors.New("it's already available")
	}
	v, ok, err := s.Store.FirstServer(kindOf(r.Type))
	if err != nil {
		return r, "", err
	}
	if !ok {
		return r, "", fmt.Errorf("%s isn't connected", arr.Kinds[kindOf(r.Type)])
	}
	if r.ProfileID == 0 {
		r.ProfileID, r.ProfileName = v.QualityProfileID, v.QualityProfileName
	}
	r.ServerID = v.ID
	var notes []string
	if r.Type == "movie" {
		notes, err = s.fixMovie(ctx, v, &r)
	} else {
		notes, err = s.fixSeries(ctx, v, &r)
	}
	if err != nil {
		return r, "", err
	}
	r.Status, r.Error, r.Progress = "sent", "", 0
	s.Store.UpdateRequest(r)
	s.mu.Lock()
	delete(s.checked, r.ID) // follow it again on the next pass
	s.mu.Unlock()
	s.Publish(r)
	s.Log.Info("request fixed", "title", r.Title, "done", strings.Join(notes, " "))
	return r, strings.Join(notes, " "), nil
}

func (s *Service) fixMovie(ctx context.Context, v store.Server, r *store.Request) ([]string, error) {
	c := client(v)
	var notes []string
	var movie map[string]any
	err := errors.New("no id")
	if r.ArrID > 0 {
		err = c.Get(ctx, fmt.Sprintf("movie/%d", r.ArrID), &movie)
	}
	if err != nil && r.ArrID > 0 && !errors.Is(err, arr.ErrNotFound) {
		return nil, err // Radarr didn't answer: nothing to fix from here
	}
	if err != nil { // not under the ID it had: look for it by title, or add it again
		var have []map[string]any
		if err := c.Get(ctx, fmt.Sprintf("movie?tmdbId=%d", r.TmdbID), &have); err != nil {
			return nil, err
		}
		if len(have) == 0 {
			if err := s.sendMovie(ctx, v, r); err != nil {
				return nil, err
			}
			notes = append(notes, "It was missing from Radarr, so it was added again.")
			if v.SearchOnAdd {
				return append(notes, "Radarr is searching for it."), nil
			}
			return append(notes, s.searchMovie(ctx, c, r.ArrID)), nil
		}
		movie, r.ArrID = have[0], num(have[0]["id"])
		notes = append(notes, "It was in Radarr under another entry; the request now follows that one.")
	}
	if on, _ := movie["monitored"].(bool); !on {
		movie["monitored"] = true
		if err := c.Put(ctx, fmt.Sprintf("movie/%d", r.ArrID), movie, nil); err != nil {
			return nil, fmt.Errorf("Radarr didn't turn it back on: %w", err)
		}
		notes = append(notes, "It wasn't monitored in Radarr; now it is.")
	}
	return append(notes, s.searchMovie(ctx, c, r.ArrID)), nil
}

func (s *Service) searchMovie(ctx context.Context, c *arr.Client, id int) string {
	if err := c.Post(ctx, "command", map[string]any{"name": "MoviesSearch", "movieIds": []int{id}}, nil); err != nil {
		return "Radarr didn't start a search: " + err.Error() + "."
	}
	return "Started a search."
}

func (s *Service) fixSeries(ctx context.Context, v store.Server, r *store.Request) ([]string, error) {
	c := client(v)
	var notes []string
	var sr map[string]any
	err := errors.New("no id")
	if r.ArrID > 0 {
		err = c.Get(ctx, fmt.Sprintf("series/%d", r.ArrID), &sr)
	}
	if err != nil && r.ArrID > 0 && !errors.Is(err, arr.ErrNotFound) {
		return nil, err
	}
	if err != nil {
		found, err := seriesLookup(ctx, c, r.TmdbID)
		if err != nil {
			return nil, err
		}
		if len(found) == 0 || num(found[0]["id"]) == 0 { // not in Sonarr at all: add it again
			if err := s.sendSeries(ctx, v, r); err != nil {
				return nil, err
			}
			notes = append(notes, "It was missing from Sonarr, so it was added again.")
			if v.SearchOnAdd || r.Mode == "episodes" {
				return append(notes, "Sonarr is searching for it."), nil
			}
			return append(notes, s.searchSeries(ctx, c, r)), nil
		}
		r.ArrID = num(found[0]["id"])
		notes = append(notes, "It was in Sonarr under another entry; the request now follows that one.")
		if err := c.Get(ctx, fmt.Sprintf("series/%d", r.ArrID), &sr); err != nil {
			return nil, err
		}
	}
	if r.Mode == "episodes" {
		if err := s.monitorEpisodes(ctx, c, v, r.ArrID, r.Episodes); err != nil {
			return nil, err
		}
		return append(notes, s.searchSeries(ctx, c, r)), nil
	}
	want := map[int]bool{}
	for _, n := range r.Seasons {
		want[n] = true
	}
	changed := sr["monitored"] != true
	if seasons, ok := sr["seasons"].([]any); ok {
		for _, x := range seasons {
			m, _ := x.(map[string]any)
			n := num(m["seasonNumber"])
			if on, _ := m["monitored"].(bool); n > 0 && !on && (r.Mode == "all" || want[n]) {
				m["monitored"], changed = true, true
			}
		}
	}
	if r.Mode == "all" && sr["monitorNewItems"] != "all" {
		sr["monitorNewItems"], changed = "all", true
	}
	if changed {
		sr["monitored"] = true
		if err := c.Put(ctx, fmt.Sprintf("series/%d", r.ArrID), sr, nil); err != nil {
			return nil, fmt.Errorf("Sonarr didn't update the series: %w", err)
		}
		notes = append(notes, "Some of what was asked for wasn't monitored in Sonarr; now it is.")
	}
	return append(notes, s.searchSeries(ctx, c, r)), nil
}

// searchSeries asks Sonarr to search for what the request asked: the whole show, its seasons, or its episodes.
func (s *Service) searchSeries(ctx context.Context, c *arr.Client, r *store.Request) string {
	var err error
	switch r.Mode {
	case "seasons":
		for _, n := range r.Seasons {
			if e := c.Post(ctx, "command", map[string]any{"name": "SeasonSearch", "seriesId": r.ArrID, "seasonNumber": n}, nil); e != nil {
				err = e
			}
		}
	case "episodes":
		eps, e := episodeList(ctx, c, r.ArrID, false)
		ids := []int{}
		asked := map[[2]int]bool{}
		for _, p := range r.Episodes {
			asked[p] = true
		}
		for _, x := range eps {
			if asked[[2]int{num(x["seasonNumber"]), num(x["episodeNumber"])}] {
				ids = append(ids, num(x["id"]))
			}
		}
		if err = e; err == nil {
			err = c.Post(ctx, "command", map[string]any{"name": "EpisodeSearch", "episodeIds": ids}, nil)
		}
	default:
		err = c.Post(ctx, "command", map[string]any{"name": "SeriesSearch", "seriesId": r.ArrID}, nil)
	}
	if err != nil {
		return "Sonarr didn't start a search: " + err.Error() + "."
	}
	return "Started a search."
}
