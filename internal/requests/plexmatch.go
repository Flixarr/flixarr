package requests

import (
	"context"
	"errors"
	"fmt"

	"github.com/Flixarr/flixarr/internal/plexlib"
	"github.com/Flixarr/flixarr/internal/store"
)

// wrongMatch says whether Plex holds a movie's file (a path as Radarr gives it) but matched it to another
// movie, so Flixarr never sees the movie in Plex (#171).
func (s *Service) wrongMatch(file string, tmdbID int) (plexlib.FileItem, bool) {
	if s.Plex == nil || !s.Plex.Connected() || inPlex(s.Store, "movie", tmdbID) {
		return plexlib.FileItem{}, false
	}
	it, ok := s.Plex.FileItem(file)
	return it, ok && it.TmdbID != tmdbID
}

// FixMatch tells Plex which movie a request's file really is, when Plex matched it to another one.
func (s *Service) FixMatch(ctx context.Context, r store.Request) error {
	if r.Type != "movie" {
		return errors.New("only movies can be matched again")
	}
	v, ok, err := s.Store.FirstServer("radarr")
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("Radarr isn't connected")
	}
	var m map[string]any
	if err := client(v).Get(ctx, fmt.Sprintf("movie/%d", r.ArrID), &m); err != nil {
		return err
	}
	f, _ := m["movieFile"].(map[string]any)
	file, _ := f["path"].(string)
	it, ok := s.wrongMatch(file, r.TmdbID)
	if !ok {
		return errors.New("Plex doesn't have it matched to another movie")
	}
	return s.Plex.FixMatch(ctx, it.RatingKey, r.TmdbID)
}
