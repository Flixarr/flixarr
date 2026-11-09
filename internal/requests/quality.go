package requests

import (
	"context"
	"fmt"
)

// SeasonQuality counts one season's episodes that have a file and how many are below the profile's goal.
type SeasonQuality struct {
	Unmet int `json:"unmet"`
	Files int `json:"files"`
}

// Quality says whether a title's files have reached the quality its profile aims for.
type Quality struct {
	Unmet   bool                  `json:"unmet"` // a movie, or a series with any episode below it
	Count   int                   `json:"count"` // series: episodes below it
	Files   int                   `json:"files"` // series: episodes with a file
	Seasons map[int]SeasonQuality `json:"seasons"`
}

// QualityFor reads it from Radarr or Sonarr's own flag. A title that isn't there has nothing to report.
func (s *Service) QualityFor(ctx context.Context, typ string, tmdbID int) Quality {
	q := Quality{Seasons: map[int]SeasonQuality{}}
	v, ok, _ := s.Store.FirstServer(kindOf(typ))
	if !ok {
		return q
	}
	c := client(v)
	found, err := s.inArr(ctx, c, typ, tmdbID)
	if err != nil || found == nil {
		return q
	}
	if typ == "movie" {
		if f, ok := found["movieFile"].(map[string]any); ok {
			q.Unmet, _ = f["qualityCutoffNotMet"].(bool)
		}
		return q
	}
	id := num(found["id"])
	var files []struct {
		ID    int  `json:"id"`
		Unmet bool `json:"qualityCutoffNotMet"`
	}
	var eps []struct {
		Season int `json:"seasonNumber"`
		FileID int `json:"episodeFileId"`
	}
	if c.Get(ctx, fmt.Sprintf("episodefile?seriesId=%d", id), &files) != nil || c.Get(ctx, fmt.Sprintf("episode?seriesId=%d", id), &eps) != nil {
		return q
	}
	unmet := map[int]bool{}
	for _, f := range files {
		unmet[f.ID] = f.Unmet
	}
	for _, e := range eps {
		if e.FileID == 0 || e.Season == 0 {
			continue
		}
		sq := q.Seasons[e.Season]
		sq.Files++
		q.Files++
		if unmet[e.FileID] {
			sq.Unmet++
			q.Count++
		}
		q.Seasons[e.Season] = sq
	}
	q.Unmet = q.Count > 0
	return q
}
