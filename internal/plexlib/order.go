package plexlib

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/Flixarr/flixarr/internal/store"
)

// Episodes mismatch (#174): some shows are numbered differently on TMDB and TheTVDB (The Office's hour
// long two parters are one episode on TMDB, two on TheTVDB). Sonarr names files by TheTVDB, so a show Plex
// orders by TMDB can end up with episodes in the wrong place or missing.

type prefs struct {
	Setting []struct {
		ID    string `json:"id"`
		Value any    `json:"value"`
	} `json:"Setting"`
}

func (p prefs) value(id string) string {
	for _, s := range p.Setting {
		if s.ID == id {
			v, _ := s.Value.(string)
			return v
		}
	}
	return ""
}

// showOrdering is how Plex orders a show's episodes: the show's own setting, else its library's.
func (l *Library) showOrdering(ctx context.Context, base, tok string, it store.PlexItem) (string, error) {
	var show struct {
		MediaContainer struct {
			Metadata []struct {
				Preferences prefs `json:"Preferences"`
			} `json:"Metadata"`
		} `json:"MediaContainer"`
	}
	if err := l.get(ctx, base, "/library/metadata/"+url.PathEscape(it.RatingKey)+"?includePreferences=1", tok, &show); err != nil {
		return "", err
	}
	if len(show.MediaContainer.Metadata) > 0 {
		if v := show.MediaContainer.Metadata[0].Preferences.value("showOrdering"); v != "" {
			return v, nil
		}
	}
	var lib struct {
		MediaContainer prefs `json:"MediaContainer"`
	}
	if err := l.get(ctx, base, "/library/sections/"+url.PathEscape(it.Section)+"/prefs", tok, &lib); err != nil {
		return "", err
	}
	return lib.MediaContainer.value("showOrdering"), nil
}

// OrderMismatch says whether Plex orders a show by TMDB while some season has a different number of
// episodes on TMDB than in Sonarr (sonarr: episodes per season there).
func (l *Library) OrderMismatch(ctx context.Context, tmdbID int, sonarr map[int]int) bool {
	base, tok, ok := l.conn()
	if !ok || l.Meta == nil {
		return false
	}
	items, err := l.Store.PlexItemsFor([]string{fmt.Sprintf("series:%d", tmdbID)})
	if err != nil || len(items) == 0 {
		return false
	}
	it := items[fmt.Sprintf("series:%d", tmdbID)]
	if order, err := l.showOrdering(ctx, base, tok, it); err != nil || order != "tmdbAiring" {
		return false
	}
	var sp seriesPage
	if err := l.metaJSON(ctx, fmt.Sprintf("series/%d", tmdbID), &sp); err != nil {
		return false
	}
	for _, s := range sp.Seasons {
		if n, ok := sonarr[s.N]; ok && s.N > 0 && n > 0 && s.Episodes > 0 && n != s.Episodes {
			return true
		}
	}
	return false
}

// UseSonarrOrder makes Plex order a show's episodes by TheTVDB, like Sonarr, and reads the show again.
func (l *Library) UseSonarrOrder(ctx context.Context, tmdbID int) error {
	base, tok, ok := l.conn()
	if !ok {
		return errors.New("no Plex server is connected")
	}
	items, err := l.Store.PlexItemsFor([]string{fmt.Sprintf("series:%d", tmdbID)})
	if err != nil {
		return err
	}
	it, ok := items[fmt.Sprintf("series:%d", tmdbID)]
	if !ok {
		return errors.New("the show isn't in Plex")
	}
	item := "/library/metadata/" + url.PathEscape(it.RatingKey)
	var out any
	if err := l.do(ctx, http.MethodPut, base, item+"/prefs?showOrdering=tvdbAiring", tok, &out); err != nil {
		return err
	}
	return l.do(ctx, http.MethodPut, base, item+"/refresh", tok, &out)
}
