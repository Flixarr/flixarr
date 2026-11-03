package requests

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/Flixarr/flixarr/internal/arr"
	"github.com/Flixarr/flixarr/internal/plexlib"
	"github.com/Flixarr/flixarr/internal/store"
)

// TMDB and TheTVDB numbering (#197). Sonarr numbers episodes by TheTVDB, Flixarr's titles come from TMDB,
// and the two don't always agree: TheTVDB splits The Office's hour long episodes in two, and keeps
// 1000-lb Sisters' "Clapback" as a special where TMDB has it as season 6 episode 11. Counts come from
// Sonarr once a show is there, and a season request also brings the specials TMDB counts in it.

const countsFor = 2 * time.Minute // how long Sonarr's series list is kept for counts

// SeasonCounts is a series' episode counts per season in Sonarr (specials left out), nil when it isn't
// there. Sonarr's whole series list is read once and kept a couple of minutes, so a page of posters or
// the whole series check costs one call.
func (s *Service) SeasonCounts(ctx context.Context, tmdbID int) map[int]plexlib.SeasonCount {
	s.mu.Lock()
	fresh := s.counts != nil && time.Since(s.countsAt) < countsFor
	all := s.counts
	s.mu.Unlock()
	if !fresh {
		v, ok, err := s.Store.FirstServer("sonarr")
		if err != nil || !ok {
			return nil
		}
		var list []map[string]any
		if err := client(v).Get(ctx, "series", &list); err != nil {
			return all[tmdbID] // keep the last counts while Sonarr doesn't answer
		}
		all = map[int]map[int]plexlib.SeasonCount{}
		for _, sr := range list {
			if id := num(sr["tmdbId"]); id > 0 {
				all[id] = seasonCounts(sr)
			}
		}
		s.mu.Lock()
		s.counts, s.countsAt = all, time.Now()
		s.mu.Unlock()
	}
	return all[tmdbID]
}

// seasonCounts reads the episode counts per season from a Sonarr series.
func seasonCounts(sr map[string]any) map[int]plexlib.SeasonCount {
	out := map[int]plexlib.SeasonCount{}
	list, _ := sr["seasons"].([]any)
	for _, x := range list {
		m, _ := x.(map[string]any)
		n := num(m["seasonNumber"])
		if n == 0 {
			continue
		}
		st, _ := m["statistics"].(map[string]any)
		c := plexlib.SeasonCount{Total: num(st["totalEpisodeCount"])}
		switch {
		case st["previousAiring"] == nil: // not started
		case st["nextAiring"] != nil: // still airing: Sonarr counts the ones out so far (monitored or with a file)
			c.Aired = min(num(st["episodeCount"]), c.Total)
		default:
			c.Aired = c.Total
		}
		out[n] = c
	}
	return out
}

var (
	partRe  = regexp.MustCompile(`\s*\((\d+|part \d+)\)$|\s*,?\s*part \d+$`)
	punctRe = regexp.MustCompile(`[^a-z0-9 ]+`)
)

// sameTitle makes episode titles comparable across TMDB and TheTVDB: "Goodbye, Toby (1)" and
// "Goodbye Toby", "The Chair Model" and "Chair Model".
func sameTitle(t string) string {
	t = strings.ToLower(strings.TrimSpace(t))
	t = partRe.ReplaceAllString(t, "")
	t = punctRe.ReplaceAllString(t, " ")
	t = strings.Join(strings.Fields(t), " ")
	return strings.TrimPrefix(t, "the ")
}

// specialsIn picks the Sonarr specials (season 0) that TMDB counts as part of a season: a TMDB episode
// whose title isn't one of Sonarr's in that season, but is one of its specials. tmdb: the season's
// episode titles on TMDB; sonarr: Sonarr's titles in that season; specials: Sonarr's specials by id.
func specialsIn(tmdb, sonarr []string, specials map[int]string) []int {
	in := map[string]bool{}
	for _, t := range sonarr {
		in[sameTitle(t)] = true
	}
	byTitle := map[string]int{}
	for id, t := range specials {
		if k := sameTitle(t); k != "" {
			byTitle[k] = id
		}
	}
	out := []int{}
	for _, t := range tmdb {
		k := sameTitle(t)
		if id, ok := byTitle[k]; ok && k != "" && !in[k] {
			out = append(out, id)
			delete(byTitle, k)
		}
	}
	return out
}

// monitorSpecials turns on (and, if the server searches on request, searches for) the specials TMDB
// counts in the requested seasons (all: every season). Best effort: a request goes through without it.
func (s *Service) monitorSpecials(ctx context.Context, c *arr.Client, v store.Server, seriesID, tmdbID int, all bool, want map[int]bool) {
	if s.Meta == nil {
		return
	}
	eps, err := episodeList(ctx, c, seriesID, true)
	if err != nil {
		return
	}
	titles := map[int][]string{}
	specials := map[int]string{}
	for _, e := range eps {
		title, _ := e["title"].(string)
		n := num(e["seasonNumber"])
		if n == 0 {
			if on, _ := e["monitored"].(bool); !on {
				specials[num(e["id"])] = title
			}
			continue
		}
		titles[n] = append(titles[n], title)
	}
	if len(specials) == 0 {
		return
	}
	ids := []int{}
	for n := range titles {
		if !all && !want[n] {
			continue
		}
		var season struct {
			Episodes []struct {
				Name string `json:"name"`
			} `json:"episodes"`
		}
		body, status, err := s.Meta.Get(ctx, fmt.Sprintf("series/%d/season/%d", tmdbID, n), url.Values{})
		if err != nil || status != http.StatusOK || json.Unmarshal(body, &season) != nil {
			continue
		}
		tmdb := make([]string, 0, len(season.Episodes))
		for _, e := range season.Episodes {
			tmdb = append(tmdb, e.Name)
		}
		found := specialsIn(tmdb, titles[n], specials)
		for _, id := range found {
			delete(specials, id)
		}
		ids = append(ids, found...)
	}
	if len(ids) == 0 {
		return
	}
	if err := c.Put(ctx, "episode/monitor", map[string]any{"episodeIds": ids, "monitored": true}, nil); err != nil {
		s.Log.Warn("specials in a season", "tmdbId", tmdbID, "err", err)
		return
	}
	if v.SearchOnAdd {
		c.Post(ctx, "command", map[string]any{"name": "EpisodeSearch", "episodeIds": ids}, nil)
	}
}

// followSpecials runs monitorSpecials on its own: right after a series is added Sonarr takes a few
// seconds to list its episodes, and the request shouldn't wait for it.
func (s *Service) followSpecials(ctx context.Context, c *arr.Client, v store.Server, seriesID, tmdbID int, all bool, want map[int]bool) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
	go func() {
		defer cancel()
		s.monitorSpecials(ctx, c, v, seriesID, tmdbID, all, want)
	}()
}
