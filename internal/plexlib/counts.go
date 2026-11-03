package plexlib

import (
	"context"
	"fmt"
	"time"
)

// Episode counts that match Plex (#197): TMDB and TheTVDB don't always number a show the same way
// (1000-lb Sisters' "Clapback" is season 6 episode 11 on TMDB, a special on TheTVDB; The Office's hour
// long episodes are one on TMDB, two on TheTVDB). When Plex numbers a show by TheTVDB, Sonarr's counts
// per season are the ones that line up with Plex's, so they're used once the show is in Sonarr.

// SeasonCount is how many episodes a season has in Sonarr, and how many of them are out so far.
type SeasonCount struct {
	Total int
	Aired int
}

type orderSeen struct {
	order string
	at    time.Time
}

const orderFor = 10 * time.Minute // how long a show's Plex ordering is kept before it's read again

// SonarrCounts is a series' episode counts per season as Sonarr has them, when Plex numbers the show
// the way Sonarr does (TheTVDB, aired order). Nil otherwise: the show isn't in Plex or Sonarr, or Plex
// numbers it by TMDB, so TMDB's counts are the right ones.
func (l *Library) SonarrCounts(ctx context.Context, tmdbID int) map[int]SeasonCount {
	if l.Sonarr == nil {
		return nil
	}
	base, tok, ok := l.conn()
	if !ok {
		return nil
	}
	key := fmt.Sprintf("series:%d", tmdbID)
	items, err := l.Store.PlexItemsFor([]string{key})
	if err != nil || len(items) == 0 {
		return nil
	}
	it := items[key]
	l.ordersMu.Lock()
	seen, ok := l.orders[it.RatingKey]
	l.ordersMu.Unlock()
	if !ok || time.Since(seen.at) > orderFor {
		order, err := l.showOrdering(ctx, base, tok, it)
		if err != nil {
			return nil
		}
		seen = orderSeen{order: order, at: time.Now()}
		l.ordersMu.Lock()
		if l.orders == nil {
			l.orders = map[string]orderSeen{}
		}
		l.orders[it.RatingKey] = seen
		l.ordersMu.Unlock()
	}
	if seen.order != "aired" { // TheTVDB (aired) is Sonarr's numbering; TMDB, DVD and absolute orders aren't
		return nil
	}
	return l.Sonarr(ctx, tmdbID)
}
