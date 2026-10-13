package server

import (
	"encoding/json"
	"net/http"
	"slices"

	"github.com/Flixarr/flixarr/internal/store"
)

// discoverRowKeys are the Discover rows, in their default order.
var discoverRowKeys = []string{"recently-added", "recent-requests", "leaving-soon", "trending", "popular-movies", "upcoming-movies", "popular-series", "upcoming-series"}

type discoverRow struct {
	Key string `json:"key"`
	On  bool   `json:"on"`
}

// discoverRows is the saved order and on/off of the Discover rows; rows added in later versions are
// appended (on), unknown ones dropped.
func (s *Server) discoverRows() []discoverRow {
	var saved []discoverRow
	json.Unmarshal([]byte(s.Store.Setting("discover_rows", "")), &saved)
	out := []discoverRow{}
	for _, r := range saved {
		if slices.Contains(discoverRowKeys, r.Key) && !slices.ContainsFunc(out, func(x discoverRow) bool { return x.Key == r.Key }) {
			out = append(out, r)
		}
	}
	for _, k := range discoverRowKeys {
		if !slices.ContainsFunc(out, func(x discoverRow) bool { return x.Key == k }) {
			out = append(out, discoverRow{Key: k, On: true})
		}
	}
	return out
}

func (s *Server) discoverRowsGet(w http.ResponseWriter, r *http.Request, _ *store.User) {
	writeJSON(w, http.StatusOK, s.discoverRows())
}

// discoverRowsSet saves Settings → Discover (admins).
func (s *Server) discoverRowsSet(w http.ResponseWriter, r *http.Request, _ *store.User) {
	var in []discoverRow
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "Bad request.")
		return
	}
	b, _ := json.Marshal(in)
	s.Store.SetSetting("discover_rows", string(b))
	writeJSON(w, http.StatusOK, s.discoverRows())
}
