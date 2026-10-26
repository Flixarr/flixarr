package requests

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/Flixarr/flixarr/internal/arr"
)

// Fixing a download stuck in Radarr's or Sonarr's queue (#129). The ways out:
//
//	different: remove it from the queue and the download client, block that release so it isn't grabbed
//	           again, and let Radarr/Sonarr search for another (what fixes most stuck downloads)
//	retry:     ask Radarr/Sonarr to try importing its files again (when they weren't ready yet)
//	force:     import the files anyway, as Radarr's / Sonarr's manual import does (when it refused them)
//	remove:    remove it from the queue and the download client without searching again
var QueueActions = map[string]string{"different": "Got a different release", "retry": "Tried the import again", "force": "Imported it anyway", "remove": "Removed from the download queue"}

// Fixed is the title a fixed download belonged to.
type Fixed struct {
	Type   string
	TmdbID int
	ArrID  int
}

// FixDownload does one of QueueActions to a queue item in the Radarr ("radarr") or Sonarr ("sonarr") connection.
func (s *Service) FixDownload(ctx context.Context, kind string, queueID int, action string) (Fixed, error) {
	if _, ok := QueueActions[action]; !ok {
		return Fixed{}, errors.New("that isn't something Flixarr can do to a download")
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
	case "force":
		err = forceImport(ctx, c, kind, download, f.ArrID)
	}
	return f, err
}

// forceImport is Radarr's / Sonarr's manual import of a download it refused (like "not an upgrade"): the
// files go in anyway, replacing what's there.
func forceImport(ctx context.Context, c *arr.Client, kind, download string, arrID int) error {
	if download == "" {
		return errors.New("Radarr or Sonarr doesn't say which download it is")
	}
	var items []map[string]any
	if err := c.Get(ctx, "manualimport?downloadId="+url.QueryEscape(download)+"&filterExistingFiles=false", &items); err != nil {
		return err
	}
	var files []map[string]any
	for _, it := range items {
		file := map[string]any{"path": it["path"], "quality": it["quality"], "languages": it["languages"], "releaseGroup": it["releaseGroup"], "indexerFlags": it["indexerFlags"], "downloadId": download}
		if kind == "sonarr" {
			var ids []int
			eps, _ := it["episodes"].([]any)
			for _, e := range eps {
				if m, ok := e.(map[string]any); ok {
					ids = append(ids, num(m["id"]))
				}
			}
			if len(ids) == 0 {
				continue // a sample or an extra Sonarr can't place
			}
			file["seriesId"], file["episodeIds"], file["releaseType"] = arrID, ids, it["releaseType"]
		} else {
			file["movieId"] = arrID
		}
		files = append(files, file)
	}
	if len(files) == 0 {
		return errors.New("there's no file in that download to import")
	}
	return c.Post(ctx, "command", map[string]any{"name": "ManualImport", "files": files, "importMode": "auto"}, nil)
}
