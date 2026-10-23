package requests

import (
	"context"
	"errors"
	"fmt"

	"github.com/Flixarr/flixarr/internal/arr"
)

// Fixing a download stuck in Radarr's or Sonarr's queue (#129). The three ways out:
//
//	different: remove it from the queue and the download client, block that release so it isn't grabbed
//	           again, and let Radarr/Sonarr search for another (what fixes most stuck downloads)
//	retry:     ask Radarr/Sonarr to try importing its files again (when they weren't ready yet)
//	remove:    remove it from the queue and the download client without searching again
var QueueActions = map[string]string{"different": "Got a different release", "retry": "Tried the import again", "remove": "Removed from the download queue"}

// Fixed is the title a fixed download belonged to.
type Fixed struct {
	Type   string
	TmdbID int
	ArrID  int
}

// FixDownload does one of QueueActions to a queue item in the Radarr ("radarr") or Sonarr ("sonarr") connection.
func (s *Service) FixDownload(ctx context.Context, kind string, queueID int, action string) (Fixed, error) {
	if _, ok := QueueActions[action]; !ok {
		return Fixed{}, errors.New("pick a different release, try the import again, or remove it")
	}
	v, ok, err := s.Store.FirstServer(kind)
	if err != nil || !ok {
		return Fixed{}, fmt.Errorf("%s isn't connected", arr.Kinds[kind])
	}
	c := &arr.Client{Kind: v.Kind, URL: v.URL, APIKey: v.APIKey}
	include := "includeMovie=true"
	if kind == "sonarr" {
		include = "includeSeries=true"
	}
	var page struct {
		Records []struct {
			ID         int    `json:"id"`
			MovieID    int    `json:"movieId"`
			SeriesID   int    `json:"seriesId"`
			OutputPath string `json:"outputPath"`
			DownloadID string `json:"downloadId"`
			Movie      *struct {
				TmdbID int `json:"tmdbId"`
			} `json:"movie"`
			Series *struct {
				TmdbID int `json:"tmdbId"`
			} `json:"series"`
		} `json:"records"`
	}
	if err := c.Get(ctx, "queue?pageSize=500&"+include, &page); err != nil {
		return Fixed{}, err
	}
	var f Fixed
	found := false
	path, download := "", ""
	for _, r := range page.Records {
		if r.ID != queueID {
			continue
		}
		found, path, download = true, r.OutputPath, r.DownloadID
		if r.MovieID > 0 {
			f.Type, f.ArrID = "movie", r.MovieID
			if r.Movie != nil {
				f.TmdbID = r.Movie.TmdbID
			}
		} else {
			f.Type, f.ArrID = "series", r.SeriesID
			if r.Series != nil {
				f.TmdbID = r.Series.TmdbID
			}
		}
	}
	if !found {
		return Fixed{}, errors.New("that download isn't in the queue anymore")
	}
	switch action {
	case "different":
		err = c.Delete(ctx, fmt.Sprintf("queue/%d?removeFromClient=true&blocklist=true&skipRedownload=false", queueID))
	case "remove":
		err = c.Delete(ctx, fmt.Sprintf("queue/%d?removeFromClient=true&blocklist=false&skipRedownload=true", queueID))
	case "retry":
		if path == "" {
			return f, errors.New("Radarr or Sonarr doesn't say where its files are yet; try again in a minute")
		}
		name := "DownloadedMoviesScan"
		if kind == "sonarr" {
			name = "DownloadedEpisodesScan"
		}
		err = c.Post(ctx, "command", map[string]any{"name": name, "path": path, "downloadClientId": download, "importMode": "Auto"}, nil)
	}
	return f, err
}
