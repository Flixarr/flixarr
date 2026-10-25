package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strings"
)

// Request is a movie or series someone asked for.
type Request struct {
	ID          int64    `json:"id"`
	Type        string   `json:"type"` // movie | series
	TmdbID      int      `json:"tmdbId"`
	Title       string   `json:"title"`
	Year        string   `json:"year"`
	Poster      string   `json:"poster"`
	Backdrop    string   `json:"backdrop"`
	UserID      int64    `json:"userId"`
	UserName    string   `json:"userName"`
	UserAvatar  string   `json:"userAvatar"`
	Status      string   `json:"status"`
	ServerID    int64    `json:"serverId"`
	ArrID       int      `json:"arrId"`
	ProfileID   int      `json:"profileId"`
	ProfileName string   `json:"profileName"`
	Mode        string   `json:"mode"`
	Seasons     []int    `json:"seasons"`
	Episodes    [][2]int `json:"episodes"` // mode episodes: [season, episode] pairs
	Progress    int      `json:"progress"`
	Error       string   `json:"error"`
	Anime       bool     `json:"anime"`
	SeasonCount int      `json:"seasonCount"` // seasons asked for (series), for request limits
	CreatedAt   string   `json:"createdAt"`
	UpdatedAt   string   `json:"updatedAt"`
	Source      string   `json:"source"`               // "" (someone asked) or "redownload"
	Detail      string   `json:"detail,omitempty"`     // the stored reason as Radarr / Sonarr gave it (people who manage requests); not a column
	Trouble     bool     `json:"trouble,omitempty"`    // its download is stuck in Radarr / Sonarr right now (#129); not a column
	Unreleased  bool     `json:"unreleased,omitempty"` // what was asked for isn't out yet, so it can't download (#151)
}

// OpenStatuses are the statuses of a request still in play (one per title at a time).
var OpenStatuses = []string{"pending", "approved", "sent", "grabbed", "downloading", "importing", "available"}

const requestCols = `r.id, r.type, r.tmdb_id, r.title, r.year, r.poster, r.backdrop, r.user_id, COALESCE(u.username, ''), r.status, COALESCE(r.server_id, 0), r.arr_id, r.profile_id, r.profile_name, r.mode, r.seasons, r.progress, r.error, r.anime, r.created_at, r.updated_at, r.episodes, r.source, COALESCE(u.avatar, ''), r.unreleased`

func scanRequest(row interface{ Scan(...any) error }) (Request, error) {
	var r Request
	var seasons, episodes string
	err := row.Scan(&r.ID, &r.Type, &r.TmdbID, &r.Title, &r.Year, &r.Poster, &r.Backdrop, &r.UserID, &r.UserName, &r.Status, &r.ServerID, &r.ArrID, &r.ProfileID, &r.ProfileName, &r.Mode, &seasons, &r.Progress, &r.Error, &r.Anime, &r.CreatedAt, &r.UpdatedAt, &episodes, &r.Source, &r.UserAvatar, &r.Unreleased)
	if err == nil {
		r.Seasons, r.Episodes = []int{}, [][2]int{}
		json.Unmarshal([]byte(seasons), &r.Seasons)
		json.Unmarshal([]byte(episodes), &r.Episodes)
	}
	return r, err
}

// CreateRequest saves a new request and returns it.
func (s *Store) CreateRequest(r Request) (Request, error) {
	t := now()
	if r.Seasons == nil {
		r.Seasons = []int{}
	}
	seasons, _ := json.Marshal(r.Seasons)
	if r.Episodes == nil {
		r.Episodes = [][2]int{}
	}
	episodes, _ := json.Marshal(r.Episodes)
	var server any
	if r.ServerID != 0 {
		server = r.ServerID
	}
	res, err := s.db.Exec(`INSERT INTO requests (type, tmdb_id, title, year, poster, backdrop, user_id, status, server_id, profile_id, profile_name, mode, seasons, anime, season_count, episodes, created_at, updated_at, source, arr_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.Type, r.TmdbID, r.Title, r.Year, r.Poster, r.Backdrop, r.UserID, r.Status, server, r.ProfileID, r.ProfileName, r.Mode, string(seasons), r.Anime, r.SeasonCount, string(episodes), t, t, r.Source, r.ArrID)
	if err != nil {
		return Request{}, err
	}
	id, _ := res.LastInsertId()
	first := "requested"
	if r.Source == "redownload" {
		first = "redownload"
	}
	s.addEvent(id, first, "", t)
	if r.Status != "pending" && r.Status != "approved" {
		s.addEvent(id, r.Status, r.Error, t)
	}
	return s.Request(id)
}

// Request returns one request.
func (s *Store) Request(id int64) (Request, error) {
	r, err := scanRequest(s.db.QueryRow(`SELECT `+requestCols+` FROM requests r LEFT JOIN users u ON u.id = r.user_id WHERE r.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Request{}, ErrNotFound
	}
	return r, err
}

// Requests lists requests newest first; userID 0 = everyone's.
func (s *Store) Requests(userID int64, limit int) ([]Request, error) {
	q := `SELECT ` + requestCols + ` FROM requests r LEFT JOIN users u ON u.id = r.user_id`
	args := []any{}
	if userID != 0 {
		q += ` WHERE r.user_id = ?`
		args = append(args, userID)
	}
	q += ` ORDER BY r.id DESC LIMIT ?`
	args = append(args, limit)
	return s.queryRequests(q, args...)
}

// OpenRequest returns the request still in play for a title, if any.
func (s *Store) OpenRequest(typ string, tmdbID int) (Request, bool, error) {
	r, err := scanRequest(s.db.QueryRow(`SELECT `+requestCols+` FROM requests r LEFT JOIN users u ON u.id = r.user_id
		WHERE r.type = ? AND r.tmdb_id = ? AND r.status IN (`+inList(OpenStatuses)+`) ORDER BY r.id DESC LIMIT 1`, typ, tmdbID))
	if errors.Is(err, sql.ErrNoRows) {
		return Request{}, false, nil
	}
	return r, err == nil, err
}

// OpenRequests lists every request still in play for a title, oldest first. A series can have several,
// one per batch of seasons.
func (s *Store) OpenRequests(typ string, tmdbID int) ([]Request, error) {
	return s.queryRequests(`SELECT `+requestCols+` FROM requests r LEFT JOIN users u ON u.id = r.user_id
		WHERE r.type = ? AND r.tmdb_id = ? AND r.status IN (`+inList(OpenStatuses)+`) ORDER BY r.id`, typ, tmdbID)
}

// Coverage is what a series' open requests already ask for: the whole show, or these seasons.
type Coverage struct {
	Whole   bool  `json:"whole"`
	Seasons []int `json:"seasons"`
}

// SeriesCoverage adds up a series' open requests.
func (s *Store) SeriesCoverage(tmdbID int) (Coverage, error) {
	list, err := s.OpenRequests("series", tmdbID)
	return coverageOf(list), err
}

// SeriesCoverageFor is SeriesCoverage for many series in one query (status lookups for whole pages).
func (s *Store) SeriesCoverageFor(tmdbIDs []int) (map[int]Coverage, error) {
	out := map[int]Coverage{}
	if len(tmdbIDs) == 0 {
		return out, nil
	}
	ids := make([]string, len(tmdbIDs))
	for i, id := range tmdbIDs {
		ids[i] = itoa(id)
	}
	list, err := s.queryRequests(`SELECT ` + requestCols + ` FROM requests r LEFT JOIN users u ON u.id = r.user_id
		WHERE r.type = 'series' AND r.tmdb_id IN (` + strings.Join(ids, ",") + `) AND r.status IN (` + inList(OpenStatuses) + `) ORDER BY r.id`)
	if err != nil {
		return out, err
	}
	by := map[int][]Request{}
	for _, r := range list {
		by[r.TmdbID] = append(by[r.TmdbID], r)
	}
	for _, id := range tmdbIDs {
		out[id] = coverageOf(by[id])
	}
	return out, nil
}

func coverageOf(list []Request) Coverage {
	c := Coverage{Seasons: []int{}}
	seen := map[int]bool{}
	for _, r := range list {
		if r.Mode == "all" {
			c.Whole = true
		}
		for _, n := range r.Seasons {
			if !seen[n] {
				seen[n] = true
				c.Seasons = append(c.Seasons, n)
			}
		}
		for _, e := range r.Episodes { // episode requests count their seasons as asked for
			if !seen[e[0]] {
				seen[e[0]] = true
				c.Seasons = append(c.Seasons, e[0])
			}
		}
	}
	sort.Ints(c.Seasons)
	return c
}

// ActiveRequests lists requests that went to a Radarr/Sonarr and aren't available yet (the request follow loop watches them).
func (s *Store) ActiveRequests() ([]Request, error) {
	return s.queryRequests(`SELECT ` + requestCols + ` FROM requests r LEFT JOIN users u ON u.id = r.user_id WHERE r.status IN ('sent', 'grabbed', 'downloading', 'importing') AND r.arr_id != 0`)
}

func (s *Store) queryRequests(q string, args ...any) ([]Request, error) {
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Request{}
	for rows.Next() {
		r, err := scanRequest(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// RequestStatusFor returns the latest open request status for each title, keyed "movie:603".
func (s *Store) RequestStatusFor(keys []string) (map[string]Request, error) {
	out := map[string]Request{}
	if len(keys) == 0 {
		return out, nil
	}
	want := map[string]bool{}
	for _, k := range keys {
		want[k] = true
	}
	// Open requests first; a title with none shows its newest failed or declined one (so pages can say
	// so and offer Request again). Expired ones don't count: the title is simply requestable again.
	rows, err := s.db.Query(`SELECT ` + requestCols + ` FROM requests r LEFT JOIN users u ON u.id = r.user_id WHERE r.status IN (` + inList(append(append([]string{}, OpenStatuses...), "failed", "declined", "removed")) + `) ORDER BY r.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	open := map[string]bool{}
	for _, o := range OpenStatuses {
		open[o] = true
	}
	for rows.Next() {
		r, err := scanRequest(rows)
		if err != nil {
			return nil, err
		}
		k := r.Type + ":" + itoa(r.TmdbID)
		if !want[k] {
			continue
		}
		prev, has := out[k]
		switch {
		case !has:
			out[k] = r
		case !open[r.Status]:
			if !open[prev.Status] {
				out[k] = r // the newest ended one
			}
		case !open[prev.Status]:
			out[k] = r // an open one beats any ended one
		// Newest wins, except a series keeps showing a request still on its way over a finished one.
		case r.Status != "available" || prev.Status == "available":
			out[k] = r
		}
	}
	return out, rows.Err()
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

// UpdateRequest saves a request's progress fields; a new status goes on its timeline.
func (s *Store) UpdateRequest(r Request) error {
	var server any
	if r.ServerID != 0 {
		server = r.ServerID
	}
	var was string
	s.db.QueryRow(`SELECT status FROM requests WHERE id = ?`, r.ID).Scan(&was)
	t := now()
	_, err := s.db.Exec(`UPDATE requests SET status = ?, server_id = ?, arr_id = ?, profile_id = ?, profile_name = ?, progress = ?, error = ?, unreleased = ?, updated_at = ? WHERE id = ?`,
		r.Status, server, r.ArrID, r.ProfileID, r.ProfileName, r.Progress, r.Error, r.Unreleased, t, r.ID)
	if err == nil && was != "" && was != r.Status {
		s.addEvent(r.ID, r.Status, r.Error, t)
	}
	return err
}

// DecideRequest records who approved or declined a request.
func (s *Store) DecideRequest(id, userID int64, status string) error {
	t := now()
	_, err := s.db.Exec(`UPDATE requests SET status = ?, decided_by = ?, updated_at = ? WHERE id = ?`, status, userID, t, id)
	if err == nil {
		var who string
		s.db.QueryRow(`SELECT username FROM users WHERE id = ?`, userID).Scan(&who)
		s.addEvent(id, status, who, t)
	}
	return err
}

// Event is one step on a request's timeline.
type Event struct {
	Status string `json:"status"` // requested, redownload, fix, approved, declined, sent, grabbed, downloading, importing, available, failed, expired
	Note   string `json:"note"`   // the reason (failed, declined, expired), who decided (approved, declined), or what was fixed and by whom (fix)
	At     string `json:"at"`
}

func (s *Store) addEvent(requestID int64, status, note, at string) {
	s.db.Exec(`INSERT INTO request_events (request_id, status, note, at) VALUES (?, ?, ?, ?)`, requestID, status, note, at)
}

// RequestEvents is a request's timeline, oldest first.
func (s *Store) RequestEvents(requestID int64) ([]Event, error) {
	rows, err := s.db.Query(`SELECT status, note, at FROM request_events WHERE request_id = ? ORDER BY id`, requestID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.Status, &e.Note, &e.At); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// DeleteRequest removes a request.
func (s *Store) DeleteRequest(id int64) error {
	_, err := s.db.Exec(`DELETE FROM requests WHERE id = ?`, id)
	return err
}

// RequestsIn lists the requests with any of these statuses.
func (s *Store) RequestsIn(statuses ...string) ([]Request, error) {
	if len(statuses) == 0 {
		return []Request{}, nil
	}
	q := `SELECT ` + requestCols + ` FROM requests r LEFT JOIN users u ON u.id = r.user_id WHERE r.status IN (?` + strings.Repeat(", ?", len(statuses)-1) + `)`
	args := make([]any, len(statuses))
	for i, st := range statuses {
		args[i] = st
	}
	return s.queryRequests(q, args...)
}

// ActiveRequestsFor lists the active requests for one Radarr/Sonarr item (a webhook names it by its own ID).
func (s *Store) ActiveRequestsFor(typ string, arrID int) ([]Request, error) {
	return s.queryRequests(`SELECT `+requestCols+` FROM requests r LEFT JOIN users u ON u.id = r.user_id WHERE r.type = ? AND r.arr_id = ? AND r.status IN ('sent', 'grabbed', 'downloading', 'importing')`, typ, arrID)
}

// AvailableRequests lists the available requests for these titles ("movie:603" keys); with series set it
// also includes every available series request (episodes left Plex, and Plex doesn't say which show).
func (s *Store) AvailableRequests(keys []string, series bool) ([]Request, error) {
	all, err := s.queryRequests(`SELECT ` + requestCols + ` FROM requests r LEFT JOIN users u ON u.id = r.user_id WHERE r.status = 'available' AND r.arr_id != 0`)
	if err != nil {
		return nil, err
	}
	want := map[string]bool{}
	for _, k := range keys {
		want[k] = true
	}
	out := []Request{}
	for _, r := range all {
		if want[r.Type+":"+itoa(r.TmdbID)] || (series && r.Type == "series") {
			out = append(out, r)
		}
	}
	return out, nil
}

// inList quotes fixed statuses for an IN (...) clause.
func inList(list []string) string {
	q := make([]string, len(list))
	for i, v := range list {
		q[i] = "'" + v + "'"
	}
	return strings.Join(q, ", ")
}

// RequestsForTitle lists every request for a title, newest first, whatever its status.
func (s *Store) RequestsForTitle(typ string, tmdbID int) ([]Request, error) {
	return s.queryRequests(`SELECT `+requestCols+` FROM requests r LEFT JOIN users u ON u.id = r.user_id WHERE r.type = ? AND r.tmdb_id = ? ORDER BY r.id DESC`, typ, tmdbID)
}

// ClearTitle removes what Flixarr keeps about a title: its requests (with their timelines), its issues
// (with their comments), its Plex row and its Maintainerr leaving rows and keep asks. Keeps stay (they
// mirror Maintainerr's exclusions), and so do the blocklist and watchlists.
func (s *Store) ClearTitle(typ string, tmdbID int) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, q := range []string{
		`DELETE FROM leaving_told WHERE request_id IN (SELECT id FROM requests WHERE type = ? AND tmdb_id = ?)`,
		`DELETE FROM requests WHERE type = ? AND tmdb_id = ?`,
		`DELETE FROM issues WHERE type = ? AND tmdb_id = ?`,
		`DELETE FROM plex_items WHERE type = ? AND tmdb_id = ?`,
		`DELETE FROM leaving WHERE type = ? AND tmdb_id = ?`,
		`DELETE FROM keep_asks WHERE type = ? AND tmdb_id = ?`,
	} {
		if _, err := tx.Exec(q, typ, tmdbID); err != nil {
			return err
		}
	}
	return tx.Commit()
}
