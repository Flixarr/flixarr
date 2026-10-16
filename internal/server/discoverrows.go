package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/Flixarr/flixarr/internal/store"
)

// discoverRowKeys are the built-in Discover rows, in their default order. The ones in rowsStartOff
// arrive turned off (Settings → Discover turns them on).
var discoverRowKeys = []string{"recently-added", "recent-requests", "leaving-soon", "trending", "popular-movies", "upcoming-movies", "popular-series", "upcoming-series", "now-playing", "airing-today", "top-rated-movies", "top-rated-series"}
var rowsStartOff = []string{"now-playing", "airing-today", "top-rated-movies", "top-rated-series"}

// discoverRow is one row; custom rows ("custom-…") also say what they show: movies or series, an
// optional genre, and the order (popular, rating, newest).
type discoverRow struct {
	Key       string `json:"key"`
	On        bool   `json:"on"`
	Title     string `json:"title,omitempty"`
	Kind      string `json:"kind,omitempty"` // movie | series
	Genre     int    `json:"genre,omitempty"`
	GenreName string `json:"genreName,omitempty"`
	Sort      string `json:"sort,omitempty"` // popular | rating | newest
}

// custom checks a custom row and tidies it; ok=false drops it.
func (r *discoverRow) custom() bool {
	r.Title = strings.TrimSpace(r.Title)
	if len(r.Title) > 60 {
		r.Title = r.Title[:60]
	}
	if r.Sort == "" {
		r.Sort = "popular"
	}
	return strings.HasPrefix(r.Key, "custom-") && len(r.Key) <= 40 && r.Title != "" && (r.Kind == "movie" || r.Kind == "series") &&
		r.Genre >= 0 && slices.Contains([]string{"popular", "rating", "newest"}, r.Sort)
}

// discoverRows is the saved order and on/off of the Discover rows; rows added in later versions are
// appended (on), unknown ones dropped.
func (s *Server) discoverRows() []discoverRow {
	var saved []discoverRow
	json.Unmarshal([]byte(s.Store.Setting("discover_rows", "")), &saved)
	out := []discoverRow{}
	for _, r := range saved {
		if slices.ContainsFunc(out, func(x discoverRow) bool { return x.Key == r.Key }) {
			continue
		}
		if slices.Contains(discoverRowKeys, r.Key) {
			out = append(out, discoverRow{Key: r.Key, On: r.On})
		} else if r.custom() {
			out = append(out, r)
		}
	}
	for _, k := range discoverRowKeys {
		if !slices.ContainsFunc(out, func(x discoverRow) bool { return x.Key == k }) {
			out = append(out, discoverRow{Key: k, On: !slices.Contains(rowsStartOff, k)})
		}
	}
	return out
}

func (s *Server) discoverRowsGet(w http.ResponseWriter, r *http.Request, _ *store.User) {
	rows := s.discoverRows()
	if !s.maintainerrOn() { // Leaving Soon only exists with Maintainerr
		rows = slices.DeleteFunc(rows, func(x discoverRow) bool { return x.Key == "leaving-soon" })
	}
	writeJSON(w, http.StatusOK, rows)
}

// discoverRowsSet saves Settings → Discover (admins).
func (s *Server) discoverRowsSet(w http.ResponseWriter, r *http.Request, _ *store.User) {
	var in []discoverRow
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384)).Decode(&in); err != nil || len(in) > 60 {
		writeErr(w, http.StatusBadRequest, "Bad request.")
		return
	}
	for i := range in {
		if in[i].Key == "custom-new" { // a row just added in Settings gets its own key
			in[i].Key = fmt.Sprintf("custom-%d%d", time.Now().UnixMilli(), i)
		}
	}
	b, _ := json.Marshal(in)
	s.Store.SetSetting("discover_rows", string(b)) // discoverRows drops anything that doesn't check out
	writeJSON(w, http.StatusOK, s.discoverRows())
}
