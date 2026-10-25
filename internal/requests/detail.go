package requests

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/Flixarr/flixarr/internal/arr"
	"github.com/Flixarr/flixarr/internal/store"
)

// What a request's details window and a title's admin panel show, read live from Radarr / Sonarr (and Plex
// for series). Release names, folders and links are only filled in for people who manage requests.

// Download is one item in the download queue.
type Download struct {
	QueueID  int    `json:"queueId,omitempty"` // set when it hit a problem (Fix)
	Season   int    `json:"season,omitempty"`
	Episode  int    `json:"episode,omitempty"`
	Percent  int    `json:"percent"`
	TimeLeft string `json:"timeLeft,omitempty"` // "01:12:05" as Radarr / Sonarr give it
	State    string `json:"state"`              // downloading, queued, importing, warning, failed
	Size     int64  `json:"size"`
	Release  string `json:"release,omitempty"` // admins only
	Problem  string `json:"problem,omitempty"` // in plain words
	Detail   string `json:"detail,omitempty"`  // what Radarr / Sonarr said (admins only)
}

// SeasonState is a requested season: episodes with files vs episodes out, and episodes in Plex.
type SeasonState struct {
	Number int `json:"number"`
	Have   int `json:"have"`
	Total  int `json:"total"`
	InPlex int `json:"inPlex"`
}

// File is the file Radarr / Sonarr has (movies), or its totals (series).
type File struct {
	Quality string `json:"quality,omitempty"`
	Size    int64  `json:"size"`
	Release string `json:"release,omitempty"` // admins only
	Path    string `json:"path,omitempty"`    // admins only
}

// Live is what Radarr / Sonarr say about a title right now.
type Live struct {
	InArr     bool          `json:"inArr"`
	Monitored bool          `json:"monitored"`
	HasFile   bool          `json:"hasFile"`
	File      *File         `json:"file,omitempty"`
	Queue     []Download    `json:"queue"`
	Seasons   []SeasonState `json:"seasons,omitempty"`
	Link      string        `json:"link,omitempty"` // opens it in Radarr / Sonarr (admins only)
	Server    string        `json:"server"`
	Error     string        `json:"error,omitempty"` // couldn't ask Radarr / Sonarr
}

// Detail is a request's details window.
type Detail struct {
	Request store.Request `json:"request"`
	Events  []store.Event `json:"events"`
	Live    Live          `json:"live"`
	Hint    string        `json:"hint,omitempty"` // what's going on, in plain words
}

// Detail gathers a request's timeline and what Radarr / Sonarr say about it now.
func (s *Service) Detail(ctx context.Context, r store.Request, admin bool) Detail {
	d := Detail{Request: r}
	d.Events, _ = s.Store.RequestEvents(r.ID)
	var seasons []int
	if r.Type == "series" && r.Mode != "all" {
		seasons = r.Seasons
		for _, e := range r.Episodes {
			if !containsInt(seasons, e[0]) {
				seasons = append(seasons, e[0])
			}
		}
	}
	d.Live = s.LiveFor(ctx, r.Type, r.TmdbID, r.ArrID, seasons, admin)
	d.Hint = hintFor(r, d.Live)
	d.Request = ForViewer(r, admin)
	for i := range d.Events {
		if st := d.Events[i].Status; st == "failed" || st == "expired" || st == "removed" {
			d.Events[i].Note = PlainError(store.Request{Status: st, Error: d.Events[i].Note})
		}
	}
	return d
}

// LiveFor reads a title from Radarr / Sonarr (by its ID there, or found by TMDB ID when 0). For a series,
// seasons limits the seasons and downloads to those (none = all).
func (s *Service) LiveFor(ctx context.Context, typ string, tmdbID, arrID int, seasons []int, admin bool) Live {
	l := Live{Queue: []Download{}}
	v, ok, _ := s.Store.FirstServer(kindOf(typ))
	if !ok {
		l.Error = arr.Kinds[kindOf(typ)] + " isn't connected"
		return l
	}
	l.Server = v.Name
	c := client(v)
	web := strings.TrimRight(v.WebURL, "/")
	if web == "" {
		web = strings.TrimRight(v.URL, "/")
	}
	want := map[int]bool{}
	for _, n := range seasons {
		want[n] = true
	}
	if typ == "movie" {
		var m map[string]any
		err := errors.New("no id")
		if arrID > 0 {
			err = c.Get(ctx, fmt.Sprintf("movie/%d", arrID), &m)
		}
		if err != nil {
			var e error
			if m, e = inArr(ctx, c, "movie", tmdbID); e != nil || m == nil {
				if e != nil {
					l.Error = e.Error()
				}
				return l
			}
		}
		l.InArr = true
		l.Monitored, _ = m["monitored"].(bool)
		l.HasFile, _ = m["hasFile"].(bool)
		if f, ok := m["movieFile"].(map[string]any); ok && l.HasFile {
			l.File = &File{Quality: qualityName(f), Size: int64(numf(f["size"]))}
			if admin {
				l.File.Release, _ = f["sceneName"].(string)
				if l.File.Release == "" {
					l.File.Release, _ = f["relativePath"].(string)
				}
				l.File.Path, _ = m["path"].(string)
			}
		}
		if admin {
			if slug, _ := m["titleSlug"].(string); slug != "" {
				l.Link = web + "/movie/" + url.PathEscape(slug)
			}
		}
		l.Queue = queue(ctx, c, fmt.Sprintf("queue/details?movieId=%d", num(m["id"])), nil, admin)
		return l
	}
	var sr map[string]any
	err := errors.New("no id")
	if arrID > 0 {
		err = c.Get(ctx, fmt.Sprintf("series/%d", arrID), &sr)
	}
	if err != nil {
		found, e := inArr(ctx, c, "series", tmdbID)
		if e != nil || found == nil {
			if e != nil {
				l.Error = e.Error()
			}
			return l
		}
		if e := c.Get(ctx, fmt.Sprintf("series/%d", num(found["id"])), &sr); e != nil {
			l.Error = e.Error()
			return l
		}
	}
	l.InArr = true
	l.Monitored, _ = sr["monitored"].(bool)
	inPlex := map[int]int{}
	if s.Plex != nil && s.Plex.Connected() {
		inPlex, _ = s.Plex.Seasons(ctx, tmdbID)
	}
	var size int64
	if list, ok := sr["seasons"].([]any); ok {
		for _, x := range list {
			m, _ := x.(map[string]any)
			n := num(m["seasonNumber"])
			if n == 0 || (len(want) > 0 && !want[n]) {
				continue
			}
			st, _ := m["statistics"].(map[string]any)
			total := num(st["totalEpisodeCount"])
			if aired := num(st["episodeCount"]); aired > 0 && aired < total {
				total = aired // only what's out so far
			}
			have := num(st["episodeFileCount"])
			size += int64(numf(st["sizeOnDisk"]))
			if total == 0 && have == 0 {
				continue
			}
			l.Seasons = append(l.Seasons, SeasonState{Number: n, Have: have, Total: total, InPlex: inPlex[n]})
			if have > 0 {
				l.HasFile = true
			}
		}
	}
	sort.Slice(l.Seasons, func(a, b int) bool { return l.Seasons[a].Number < l.Seasons[b].Number })
	if l.HasFile {
		l.File = &File{Size: size}
		if admin {
			l.File.Path, _ = sr["path"].(string)
		}
	}
	if admin {
		if slug, _ := sr["titleSlug"].(string); slug != "" {
			l.Link = web + "/series/" + url.PathEscape(slug)
		}
	}
	var only map[int]bool
	if len(want) > 0 {
		only = want
	}
	l.Queue = queue(ctx, c, fmt.Sprintf("queue/details?seriesId=%d&includeEpisode=true", num(sr["id"])), only, admin)
	return l
}

func qualityName(f map[string]any) string {
	q, _ := f["quality"].(map[string]any)
	qq, _ := q["quality"].(map[string]any)
	n, _ := qq["name"].(string)
	return n
}

// queue reads the download queue for a title (only the given seasons, when set).
func queue(ctx context.Context, c *arr.Client, path string, seasons map[int]bool, admin bool) []Download {
	var items []map[string]any
	out := []Download{}
	if c.Get(ctx, path, &items) != nil {
		return out
	}
	for _, it := range items {
		season := num(it["seasonNumber"])
		if seasons != nil && !seasons[season] {
			continue
		}
		d := Download{Season: season, Size: int64(numf(it["size"]))}
		if ep, ok := it["episode"].(map[string]any); ok {
			d.Episode = num(ep["episodeNumber"])
		}
		if size := numf(it["size"]); size > 0 {
			d.Percent = int((size - numf(it["sizeleft"])) / size * 100)
		}
		d.TimeLeft, _ = it["timeleft"].(string)
		state, _ := it["trackedDownloadState"].(string)
		status, _ := it["status"].(string)
		health, _ := it["trackedDownloadStatus"].(string)
		switch {
		case health == "error" || status == "failed":
			d.State = "failed"
		case health == "warning":
			d.State = "warning"
		case state == "importPending" || state == "importing" || state == "imported" || status == "completed":
			d.State = "importing"
		case status == "downloading":
			d.State = "downloading"
		default:
			d.State = "queued"
		}
		if d.State == "failed" || d.State == "warning" {
			d.QueueID = num(it["id"])
			d.Problem = "Hit a problem. Someone who manages requests can sort it out."
			if admin {
				d.Detail = statusMessages(it)
			}
		}
		if admin {
			d.Release, _ = it["title"].(string)
		}
		out = append(out, d)
	}
	sort.Slice(out, func(a, b int) bool {
		if out[a].Season != out[b].Season {
			return out[a].Season < out[b].Season
		}
		return out[a].Episode < out[b].Episode
	})
	return out
}

// statusMessages joins what Radarr / Sonarr say is wrong with a download.
func statusMessages(it map[string]any) string {
	var parts []string
	if list, ok := it["statusMessages"].([]any); ok {
		for _, x := range list {
			m, _ := x.(map[string]any)
			if msgs, ok := m["messages"].([]any); ok {
				for _, s := range msgs {
					if t, _ := s.(string); t != "" {
						parts = append(parts, t)
					}
				}
			}
		}
	}
	if msg, _ := it["errorMessage"].(string); msg != "" {
		parts = append(parts, msg)
	}
	return strings.Join(parts, " ")
}

// NotReleased is the warning on a request for something that isn't out yet (#151).
const NotReleased = "Not released yet. It won't download until it's out."

// hintFor says what's going on with a request, in plain words (the same for everyone).
func hintFor(r store.Request, l Live) string {
	switch r.Status {
	case "pending":
		return "Waiting for someone who manages requests to approve it."
	case "declined":
		if r.Error != "" {
			return "Declined: " + r.Error
		}
		return "Declined."
	case "failed", "expired", "removed":
		return PlainError(r)
	case "available":
		return ""
	}
	if l.Error != "" {
		return "Can't check on it right now. It updates on its own."
	}
	for _, d := range l.Queue {
		if d.Problem != "" {
			return d.Problem
		}
	}
	switch {
	case !l.InArr || !l.Monitored && len(l.Queue) == 0 && (r.Status == "sent" || r.Status == "approved"):
		return "Something went wrong with this request. Someone who manages requests can sort it out."
	case len(l.Queue) == 0 && r.Unreleased && (r.Status == "sent" || r.Status == "approved"):
		return NotReleased
	case len(l.Queue) == 0 && (r.Status == "sent" || r.Status == "approved"):
		return "Still looking for a download. It starts as soon as one turns up."
	case r.Status == "grabbed":
		return "Found a download. It starts in a moment."
	case r.Status == "importing" && len(l.Queue) == 0:
		return "Downloaded. It shows up in Plex in a moment."
	}
	return ""
}

func containsInt(list []int, n int) bool {
	for _, x := range list {
		if x == n {
			return true
		}
	}
	return false
}

// FixTitle is Fix from a title's admin panel: its open request when there is one; otherwise, for a
// title in Radarr / Sonarr that nobody requested through Flixarr, turn monitoring on and search.
func (s *Service) FixTitle(ctx context.Context, typ string, tmdbID int) (string, error) {
	if list, _ := s.Store.OpenRequests(typ, tmdbID); len(list) > 0 {
		for i := len(list) - 1; i >= 0; i-- {
			if list[i].Status != "available" && list[i].Status != "pending" {
				_, done, err := s.Fix(ctx, list[i])
				return done, err
			}
		}
	}
	v, ok, _ := s.Store.FirstServer(kindOf(typ))
	if !ok {
		return "", fmt.Errorf("%s isn't connected", arr.Kinds[kindOf(typ)])
	}
	c := client(v)
	if typ == "movie" {
		m, err := inArr(ctx, c, "movie", tmdbID)
		if err != nil {
			return "", err
		}
		if m == nil {
			return "", errors.New("it isn't in Radarr; request it instead")
		}
		r := store.Request{Type: "movie", TmdbID: tmdbID, ArrID: num(m["id"])}
		notes, err := s.fixMovie(ctx, v, &r)
		return strings.Join(notes, " "), err
	}
	found, err := inArr(ctx, c, "series", tmdbID)
	if err != nil {
		return "", err
	}
	if found == nil {
		return "", errors.New("it isn't in Sonarr; request it instead")
	}
	// Nobody asked for particular seasons: leave which seasons are monitored as they are; make sure the
	// series itself is, and search for what's monitored.
	id := num(found["id"])
	var notes []string
	var sr map[string]any
	if err := c.Get(ctx, fmt.Sprintf("series/%d", id), &sr); err != nil {
		return "", err
	}
	if sr["monitored"] != true {
		sr["monitored"] = true
		if err := c.Put(ctx, fmt.Sprintf("series/%d", id), sr, nil); err != nil {
			return "", fmt.Errorf("Sonarr didn't turn it back on: %w", err)
		}
		notes = append(notes, "It wasn't monitored in Sonarr; now it is.")
	}
	if err := c.Post(ctx, "command", map[string]any{"name": "SeriesSearch", "seriesId": id}, nil); err != nil {
		return "", fmt.Errorf("Sonarr didn't start a search: %w", err)
	}
	return strings.Join(append(notes, "Started a search for its monitored episodes."), " "), nil
}

// PlainError is a request's reason in plain words for regular users (no Radarr, Sonarr or error codes).
// People who manage requests see the stored reason as it is.
func PlainError(r store.Request) string {
	switch r.Status {
	case "removed":
		return "Removed from Plex. Request it again to get it back."
	case "failed":
		return "It couldn't be added. Someone who manages requests can look into it."
	case "expired":
		return "Removed from Plex to make room. Request it again to get it back."
	case "declined":
		return r.Error // the admin's own words
	}
	return ""
}

// ForViewer is a request as shown: the reason in plain words for everyone, and for people who manage
// requests the stored reason as Detail.
func ForViewer(r store.Request, admin bool) store.Request {
	raw := r.Error
	r.Error = PlainError(r)
	if admin && raw != r.Error {
		r.Detail = raw
	}
	return r
}
