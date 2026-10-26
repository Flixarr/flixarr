package requests

import (
	"context"

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

// autoMatch fixes a movie Plex matched to the wrong title: Radarr's file and the request's TMDB ID say
// which movie it really is. It tries once per request; if Plex still has it wrong, the request says so.
func (s *Service) autoMatch(ctx context.Context, r *store.Request, movie map[string]any) {
	f, _ := movie["movieFile"].(map[string]any)
	file, _ := f["path"].(string)
	it, ok := s.wrongMatch(file, r.TmdbID)
	if !ok {
		return
	}
	s.mu.Lock()
	if s.matchTried == nil {
		s.matchTried = map[int64]bool{}
	}
	tried := s.matchTried[r.ID]
	s.matchTried[r.ID] = true
	s.mu.Unlock()
	if tried {
		return
	}
	if err := s.Plex.FixMatch(ctx, it.RatingKey, r.TmdbID); err != nil {
		s.Log.Warn("plex match", "title", r.Title, "err", err)
		return
	}
	s.Log.Info("fixed the match in plex", "title", r.Title, "was", it.Title)
	s.Store.AddTitleEvent(r.Type, r.TmdbID, "fix", "Fixed the match in Plex", 0, "")
	s.Store.AddFixSteps(r.Type, r.TmdbID, r.ID, "Fixed the match in Plex", 0)
}
