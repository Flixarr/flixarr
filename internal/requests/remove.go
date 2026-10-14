package requests

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/Flixarr/flixarr/internal/arr"
)

// RemoveFromArr takes a title out of Radarr/Sonarr with its files (Seerr's "Remove from Radarr"). It
// isn't added to their import exclusions, so it can be requested again. Requests for it end as failed
// with the same reason a removal found later gets. inArr=false: it wasn't there (nothing changed).
func (s *Service) RemoveFromArr(ctx context.Context, typ string, tmdbID int) (inArr bool, err error) {
	v, ok, err := s.Store.FirstServer(kindOf(typ))
	if err != nil {
		return false, err
	}
	if !ok {
		return false, fmt.Errorf("%s isn't connected", arr.Kinds[kindOf(typ)])
	}
	c := client(v)
	var found []map[string]any
	if typ == "movie" {
		err = c.Get(ctx, fmt.Sprintf("movie?tmdbId=%d", tmdbID), &found)
	} else {
		err = c.Get(ctx, "series/lookup?term="+url.QueryEscape(fmt.Sprintf("tmdb:%d", tmdbID)), &found)
	}
	if err != nil {
		return false, err
	}
	id := 0
	if len(found) > 0 {
		id = num(found[0]["id"]) // a series lookup also finds shows Sonarr doesn't have (id 0)
	}
	if id == 0 {
		return false, nil
	}
	path := fmt.Sprintf("movie/%d?deleteFiles=true&addImportExclusion=false", id)
	if typ == "series" {
		path = fmt.Sprintf("series/%d?deleteFiles=true&addImportListExclusion=false", id)
	}
	if err := c.Delete(ctx, path); err != nil && !errors.Is(err, arr.ErrNotFound) {
		return true, fmt.Errorf("%s didn't remove it: %w", arr.Kinds[kindOf(typ)], err)
	}
	s.mu.Lock()
	reqs, _ := s.Store.RequestsForTitle(typ, tmdbID)
	for _, r := range reqs {
		switch r.Status {
		case "sent", "grabbed", "downloading", "importing", "available":
			r.Status, r.Progress = "failed", 0
			r.Error = fmt.Sprintf("%s from %s. Request it again to get it back.", removedNote, arr.Kinds[kindOf(typ)])
			s.Store.UpdateRequest(r)
			s.Publish(r)
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
	}
	s.alertWaiting()
	return nil
}
