package arr

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Snapshot is what the Services card shows for one server, read by the refresh-status job.
type Snapshot struct {
	Connected     bool       `json:"connected"`
	Error         string     `json:"error,omitempty"`
	Version       string     `json:"version,omitempty"`
	Health        []string   `json:"health"`        // written-out issues, most important first
	Queue         []QueueRow `json:"queue"`         // every item, in Radarr/Sonarr's order
	Folder        string     `json:"folder"`        // the default root folder
	FreeSpace     int64      `json:"freeSpace"`     // free space on that folder's drive
	TotalSpace    int64      `json:"totalSpace"`    // 0 when the app doesn't report the drive's size
	Indexers      int        `json:"indexers"`      // enabled indexers
	IndexersDown  int        `json:"indexersDown"`  // named in a health issue
	Clients       []string   `json:"clients"`       // enabled download clients
	ClientsDown   bool       `json:"clientsDown"`   // a download client health issue is open
	UpdatedAt     time.Time  `json:"updatedAt"`     // when this was read
	LastConnected *time.Time `json:"lastConnected"` // last successful read
}

// QueueRow is one download.
type QueueRow struct {
	ID       int    `json:"id"`     // the queue item in Radarr/Sonarr (for Fix)
	Type     string `json:"type"`   // movie | series
	TmdbID   int    `json:"tmdbId"` // the title, for a link to it
	ArrID    int    `json:"arrId"`  // the movie/series in Radarr/Sonarr
	Title    string `json:"title"`
	Quality  string `json:"quality"`
	Size     int64  `json:"size"`
	Percent  int    `json:"percent"`
	TimeLeft string `json:"timeLeft"` // e.g. "4 min"; empty when not downloading
	State    string `json:"state"`    // downloading, queued, paused, importing, warning, error
	Message  string `json:"message,omitempty"`
	// RequestID is the Flixarr request a stuck download belongs to (filled in for Settings, Services).
	RequestID int64 `json:"requestId,omitempty"`
}

type queueRecord struct {
	ID                    int     `json:"id"`
	MovieID               int     `json:"movieId"`
	SeriesID              int     `json:"seriesId"`
	Title                 string  `json:"title"`
	Size                  float64 `json:"size"`
	SizeLeft              float64 `json:"sizeleft"`
	TimeLeft              string  `json:"timeleft"`
	Status                string  `json:"status"`
	TrackedDownloadStatus string  `json:"trackedDownloadStatus"`
	TrackedDownloadState  string  `json:"trackedDownloadState"`
	ErrorMessage          string  `json:"errorMessage"`
	StatusMessages        []struct {
		Messages []string `json:"messages"`
	} `json:"statusMessages"`
	Quality struct {
		Quality struct {
			Name string `json:"name"`
		} `json:"quality"`
	} `json:"quality"`
	Movie *struct {
		Title  string `json:"title"`
		Year   int    `json:"year"`
		TmdbID int    `json:"tmdbId"`
	} `json:"movie"`
	Series *struct {
		Title  string `json:"title"`
		TmdbID int    `json:"tmdbId"`
	} `json:"series"`
	Episode *struct {
		SeasonNumber  int `json:"seasonNumber"`
		EpisodeNumber int `json:"episodeNumber"`
	} `json:"episode"`
}

type healthRecord struct {
	Source  string `json:"source"`
	Type    string `json:"type"` // ok, notice, warning, error
	Message string `json:"message"`
}

type diskRecord struct {
	Path       string `json:"path"`
	FreeSpace  int64  `json:"freeSpace"`
	TotalSpace int64  `json:"totalSpace"`
}

// Read collects everything the card needs. folder is the server's default root folder.
func (c *Client) Read(ctx context.Context, folder string) Snapshot {
	snap := Snapshot{Health: []string{}, Queue: []QueueRow{}, Clients: []string{}, Folder: folder, UpdatedAt: time.Now().UTC()}
	var st Status
	if err := c.get(ctx, "system/status", &st); err != nil {
		snap.Error = capitalize(err.Error()) + "."
		return snap
	}
	snap.Connected, snap.Version = true, st.Version

	var health []healthRecord
	if c.get(ctx, "health", &health) == nil {
		sort.SliceStable(health, func(i, j int) bool { return rank(health[i].Type) > rank(health[j].Type) })
		for _, h := range health {
			if (h.Type != "warning" && h.Type != "error") || h.Message == "" {
				continue // notices are informational, not issues
			}
			snap.Health = append(snap.Health, h.Message)
			if h.Source == "IndexerStatusCheck" || h.Source == "IndexerLongTermStatusCheck" {
				snap.IndexersDown += namedIn(h.Message)
			}
			if strings.Contains(h.Source, "DownloadClient") {
				snap.ClientsDown = true
			}
		}
	}

	include := "includeMovie=true"
	if c.Kind == "sonarr" {
		include = "includeSeries=true&includeEpisode=true"
	}
	var queue struct {
		Records []queueRecord `json:"records"`
	}
	if c.get(ctx, "queue?pageSize=100&"+include, &queue) == nil {
		for _, r := range queue.Records {
			snap.Queue = append(snap.Queue, row(r))
		}
	}

	var indexers []struct {
		EnableRss             bool `json:"enableRss"`
		EnableAutomaticSearch bool `json:"enableAutomaticSearch"`
	}
	if c.get(ctx, "indexer", &indexers) == nil {
		for _, i := range indexers {
			if i.EnableRss || i.EnableAutomaticSearch {
				snap.Indexers++
			}
		}
	}
	snap.IndexersDown = min(snap.IndexersDown, snap.Indexers)
	var clients []struct {
		Name   string `json:"name"`
		Enable bool   `json:"enable"`
	}
	if c.get(ctx, "downloadclient", &clients) == nil {
		for _, d := range clients {
			if d.Enable {
				snap.Clients = append(snap.Clients, d.Name)
			}
		}
	}

	// Free space comes from the root folder. The drive's total comes from the disk list, which only
	// counts when it holds the folder's drive (same free space); Sonarr v4 leaves network shares out.
	var folders []Folder
	if c.get(ctx, "rootfolder", &folders) == nil {
		for _, f := range folders {
			if f.Path == folder || (folder == "" && snap.Folder == "") {
				snap.Folder, snap.FreeSpace = f.Path, f.FreeSpace
				break
			}
		}
	}
	var disks []diskRecord
	if snap.Folder != "" && c.get(ctx, "diskspace", &disks) == nil {
		best := ""
		for _, d := range disks {
			if within(snap.Folder, d.Path) && len(d.Path) > len(best) && sameDrive(d.FreeSpace, snap.FreeSpace) && d.TotalSpace > 0 {
				best, snap.TotalSpace = d.Path, d.TotalSpace
			}
		}
	}
	return snap
}

// sameDrive reports whether two free-space readings (taken moments apart, while downloads may be
// writing) are the same drive: within 1% or 1 GB, whichever is larger.
func sameDrive(a, b int64) bool {
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff <= max(b/100, 1<<30)
}

func rank(t string) int {
	switch t {
	case "error":
		return 2
	case "warning":
		return 1
	}
	return 0
}

// namedIn counts the names in messages like "Indexers unavailable due to failures: NZBgeek, DrunkenSlug".
func namedIn(msg string) int {
	i := strings.LastIndex(msg, ":")
	if i < 0 {
		return 1
	}
	return len(strings.Split(msg[i+1:], ","))
}

func within(path, root string) bool {
	root = strings.TrimRight(root, "/")
	return root == "" || path == root || strings.HasPrefix(path, root+"/")
}

func row(r queueRecord) QueueRow {
	q := QueueRow{ID: r.ID, Title: r.Title, Quality: r.Quality.Quality.Name, Size: int64(r.Size)}
	if r.MovieID > 0 {
		q.Type, q.ArrID = "movie", r.MovieID
		if r.Movie != nil {
			q.TmdbID = r.Movie.TmdbID
		}
	} else if r.SeriesID > 0 {
		q.Type, q.ArrID = "series", r.SeriesID
		if r.Series != nil {
			q.TmdbID = r.Series.TmdbID
		}
	}
	switch {
	case r.Movie != nil && r.Movie.Title != "":
		q.Title = r.Movie.Title
		if r.Movie.Year > 0 {
			q.Title += fmt.Sprintf(" (%d)", r.Movie.Year)
		}
	case r.Series != nil && r.Series.Title != "":
		q.Title = r.Series.Title
		if r.Episode != nil {
			q.Title += fmt.Sprintf(" S%02dE%02d", r.Episode.SeasonNumber, r.Episode.EpisodeNumber)
		}
	}
	if r.Size > 0 {
		q.Percent = int((r.Size - r.SizeLeft) / r.Size * 100)
	}
	var msgs []string
	for _, m := range r.StatusMessages {
		msgs = append(msgs, m.Messages...)
	}
	if r.ErrorMessage != "" {
		msgs = append([]string{r.ErrorMessage}, msgs...)
	}
	if len(msgs) > 0 {
		q.Message = msgs[0]
	}
	switch {
	case r.TrackedDownloadStatus == "error" || r.Status == "failed" || r.TrackedDownloadState == "failed" || r.TrackedDownloadState == "failedPending":
		q.State = "error"
	case r.TrackedDownloadStatus == "warning" || r.TrackedDownloadState == "importBlocked" || r.Status == "warning" || r.Status == "downloadClientUnavailable":
		q.State = "warning"
	case r.TrackedDownloadState == "importing" || r.TrackedDownloadState == "importPending" || r.Status == "completed":
		q.State = "importing"
	case r.Status == "paused":
		q.State = "paused"
	case r.Status == "queued" || r.Status == "delay":
		q.State = "queued"
	default:
		q.State = "downloading"
		q.TimeLeft = humanLeft(r.TimeLeft)
	}
	if q.State != "warning" && q.State != "error" {
		q.Message = ""
	}
	return q
}

// humanLeft turns "01:04:12" or "1.02:00:00" into "1 hr 4 min" / "1 day 2 hr".
func humanLeft(s string) string {
	if s == "" {
		return ""
	}
	days := 0
	if i := strings.Index(s, "."); i > 0 && strings.Count(s[:i], ":") == 0 {
		days, _ = strconv.Atoi(s[:i])
		s = s[i+1:]
	}
	p := strings.Split(s, ":")
	if len(p) != 3 {
		return ""
	}
	h, _ := strconv.Atoi(p[0])
	m, _ := strconv.Atoi(p[1])
	sec, _ := strconv.Atoi(strings.Split(p[2], ".")[0])
	switch {
	case days > 0:
		return fmt.Sprintf("%d day%s %d hr", days, plural(days), h)
	case h > 0:
		return fmt.Sprintf("%d hr %d min", h, m)
	case m > 0:
		return fmt.Sprintf("%d min", m)
	case sec > 0:
		return "under a minute"
	}
	return ""
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
