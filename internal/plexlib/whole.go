package plexlib

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// Whole or part of a series (#137): for each series in Plex, how many of the episodes aired so far Plex
// has. Seasons that haven't started don't count; for a season still airing, only its episodes already out.
// Specials (season 0) are left out. Posters show a full check for all of it and a half one for part of it.

type seriesPage struct {
	Seasons []struct {
		N        int     `json:"n"`
		Episodes int     `json:"episodes"`
		Air      *string `json:"air"`
	} `json:"seasons"`
	Next *string `json:"next"` // the next episode's air date: the show is still airing
}

type seasonPage struct {
	Episodes []struct {
		Air *string `json:"air"`
	} `json:"episodes"`
}

// EpisodeCounts checks every series in Plex and records how much of it Plex has (a job).
func (l *Library) EpisodeCounts(ctx context.Context) (string, error) {
	if _, _, ok := l.conn(); !ok || l.Meta == nil {
		return "No Plex server", nil
	}
	ids, err := l.Store.PlexSeriesIDs()
	if err != nil {
		return "", err
	}
	today := time.Now().Format("2006-01-02")
	whole, part, failed := 0, 0, 0
	for _, id := range ids {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		have, total, err := l.seriesEpisodes(ctx, id, today)
		if err != nil {
			failed++
			continue
		}
		l.Store.SetSeriesEpisodes(id, have, total)
		if have >= total {
			whole++
		} else {
			part++
		}
	}
	msg := fmt.Sprintf("%d series: %d whole, %d in part", len(ids), whole, part)
	if failed > 0 {
		msg += fmt.Sprintf(", %d couldn't be checked", failed)
	}
	return msg, nil
}

// seriesEpisodes is how many aired episodes of a series Plex has, out of how many have aired.
func (l *Library) seriesEpisodes(ctx context.Context, tmdbID int, today string) (have, total int, err error) {
	inPlex, err := l.Seasons(ctx, tmdbID)
	if err != nil {
		return 0, 0, err
	}
	var sp seriesPage
	if err := l.metaJSON(ctx, fmt.Sprintf("series/%d", tmdbID), &sp); err != nil {
		return 0, 0, err
	}
	latest := 0 // the newest season that has started
	for _, s := range sp.Seasons {
		if s.Air != nil && *s.Air <= today && s.N > latest {
			latest = s.N
		}
	}
	for _, s := range sp.Seasons {
		if s.N <= 0 || s.Air == nil || *s.Air > today {
			continue // specials, and seasons that haven't started
		}
		aired := s.Episodes
		if s.N == latest && sp.Next != nil {
			// Still airing: count only the episodes already out.
			var season seasonPage
			if err := l.metaJSON(ctx, fmt.Sprintf("series/%d/season/%d", tmdbID, s.N), &season); err != nil {
				return 0, 0, err
			}
			aired = 0
			for _, e := range season.Episodes {
				if e.Air != nil && *e.Air <= today {
					aired++
				}
			}
		}
		total += aired
		have += min(inPlex[s.N], aired)
	}
	return have, total, nil
}

func (l *Library) metaJSON(ctx context.Context, path string, out any) error {
	body, status, err := l.Meta.Get(ctx, path, url.Values{})
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("the Flixarr API answered %d", status)
	}
	return json.Unmarshal(body, out)
}
