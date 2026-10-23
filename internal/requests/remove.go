package requests

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/Flixarr/flixarr/internal/arr"
)

// ArrName is Radarr for movies, Sonarr for series.
func ArrName(typ string) string { return arr.Kinds[kindOf(typ)] }

// inArr is the title as Radarr/Sonarr have it, or nil when it isn't there. (A series lookup also finds
// shows Sonarr doesn't have; those have id 0.)
func inArr(ctx context.Context, c *arr.Client, typ string, tmdbID int) (map[string]any, error) {
	var found []map[string]any
	var err error
	if typ == "movie" {
		err = c.Get(ctx, fmt.Sprintf("movie?tmdbId=%d", tmdbID), &found)
	} else {
		found, err = seriesLookup(ctx, c, tmdbID)
	}
	if err != nil || len(found) == 0 || num(found[0]["id"]) == 0 {
		return nil, err
	}
	return found[0], nil
}

// seriesLookup asks Sonarr for a series by TMDB ID. Sonarr falls back to a TVDB ID match when its
// metadata doesn't know the TMDB ID (TMDB 71962 came back as Wings, TVDB 71962), so only results with
// that tmdbId count.
func seriesLookup(ctx context.Context, c *arr.Client, tmdbID int) ([]map[string]any, error) {
	var found []map[string]any
	if err := c.Get(ctx, "series/lookup?term="+url.QueryEscape(fmt.Sprintf("tmdb:%d", tmdbID)), &found); err != nil {
		return nil, err
	}
	out := []map[string]any{}
	for _, x := range found {
		if num(x["tmdbId"]) == tmdbID {
			out = append(out, x)
		}
	}
	return out, nil
}

// RemoveFromArr takes a title out of Radarr/Sonarr with its files (Seerr's "Remove from Radarr"). It
// isn't added to their import exclusions, so it can be requested again. Requests for it end as failed
// as removed. found=false: it wasn't there (nothing changed).
func (s *Service) RemoveFromArr(ctx context.Context, typ string, tmdbID int) (found bool, err error) {
	v, ok, err := s.Store.FirstServer(kindOf(typ))
	if err != nil {
		return false, err
	}
	if !ok {
		return false, fmt.Errorf("%s isn't connected", ArrName(typ))
	}
	c := client(v)
	item, err := inArr(ctx, c, typ, tmdbID)
	if err != nil || item == nil {
		return false, err
	}
	id := num(item["id"])
	path := fmt.Sprintf("movie/%d?deleteFiles=true&addImportExclusion=false", id)
	if typ == "series" {
		path = fmt.Sprintf("series/%d?deleteFiles=true&addImportListExclusion=false", id)
	}
	if err := c.Delete(ctx, path); err != nil && !errors.Is(err, arr.ErrNotFound) {
		return true, fmt.Errorf("%s didn't remove it: %w", ArrName(typ), err)
	}
	s.mu.Lock()
	reqs, _ := s.Store.RequestsForTitle(typ, tmdbID)
	for _, r := range reqs {
		switch r.Status {
		case "sent", "notout", "grabbed", "downloading", "importing", "available":
			r.Status, r.Progress = "removed", 0
			r.Error = fmt.Sprintf("Removed from %s.", ArrName(typ))
			s.Store.UpdateRequest(r)
			s.Publish(r)
			s.Alert(r)
		}
	}
	s.mu.Unlock()
	if s.Plex != nil && s.Plex.Connected() {
		s.Plex.Rescan(ctx, typ) // the files are gone: Plex drops the title, and so does Flixarr
	}
	s.Log.Info("removed from "+v.Kind, "type", typ, "tmdb", tmdbID)
	return true, nil
}

// ClearTitle forgets everything Flixarr knows about a title (Seerr's "Clear data"): its requests and
// their timelines, its issues and comments, its Plex and Maintainerr rows. Radarr, Sonarr, Plex and
// Maintainerr aren't touched; if it's still in Plex the next scan finds it again. The blocklist and
// people's watchlists stay.
func (s *Service) ClearTitle(typ string, tmdbID int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	reqs, _ := s.Store.RequestsForTitle(typ, tmdbID)
	if err := s.Store.ClearTitle(typ, tmdbID); err != nil {
		return err
	}
	for _, r := range reqs {
		r.Status = "deleted"
		s.Publish(r)
		s.Forget(r)
	}
	return nil
}
