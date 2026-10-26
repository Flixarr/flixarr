package requests

import (
	"context"
	"errors"
)

// UseSonarrOrder makes Plex number a series' episodes the way Sonarr does (#174).
func (s *Service) UseSonarrOrder(ctx context.Context, tmdbID int) error {
	if s.Plex == nil || !s.Plex.Connected() {
		return errors.New("no Plex server is connected")
	}
	return s.Plex.UseSonarrOrder(ctx, tmdbID)
}
