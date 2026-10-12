package store

import (
	"database/sql"
	"encoding/json"
	"errors"
)

// Request is a movie or series someone asked for.
type Request struct {
	ID          int64  `json:"id"`
	Type        string `json:"type"` // movie | series
	TmdbID      int    `json:"tmdbId"`
	Title       string `json:"title"`
	Year        string `json:"year"`
	Poster      string `json:"poster"`
	Backdrop    string `json:"backdrop"`
	UserID      int64  `json:"userId"`
	UserName    string `json:"userName"`
	Status      string `json:"status"`
	ServerID    int64  `json:"serverId"`
	ArrID       int    `json:"arrId"`
	ProfileID   int    `json:"profileId"`
	ProfileName string `json:"profileName"`
	Mode        string `json:"mode"`
	Seasons     []int  `json:"seasons"`
	Progress    int    `json:"progress"`
	Error       string `json:"error"`
	Anime       bool   `json:"anime"`
	SeasonCount int    `json:"seasonCount"` // seasons asked for (series), for request limits
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

// OpenStatuses are the statuses of a request still in play (one per title at a time).
var OpenStatuses = []string{"pending", "approved", "sent", "downloading", "available"}

const requestCols = `r.id, r.type, r.tmdb_id, r.title, r.year, r.poster, r.backdrop, r.user_id, COALESCE(u.username, ''), r.status, COALESCE(r.server_id, 0), r.arr_id, r.profile_id, r.profile_name, r.mode, r.seasons, r.progress, r.error, r.anime, r.created_at, r.updated_at`

func scanRequest(row interface{ Scan(...any) error }) (Request, error) {
	var r Request
	var seasons string
	err := row.Scan(&r.ID, &r.Type, &r.TmdbID, &r.Title, &r.Year, &r.Poster, &r.Backdrop, &r.UserID, &r.UserName, &r.Status, &r.ServerID, &r.ArrID, &r.ProfileID, &r.ProfileName, &r.Mode, &seasons, &r.Progress, &r.Error, &r.Anime, &r.CreatedAt, &r.UpdatedAt)
	if err == nil {
		r.Seasons = []int{}
		json.Unmarshal([]byte(seasons), &r.Seasons)
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
	var server any
	if r.ServerID != 0 {
		server = r.ServerID
	}
	res, err := s.db.Exec(`INSERT INTO requests (type, tmdb_id, title, year, poster, backdrop, user_id, status, server_id, profile_id, profile_name, mode, seasons, anime, season_count, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.Type, r.TmdbID, r.Title, r.Year, r.Poster, r.Backdrop, r.UserID, r.Status, server, r.ProfileID, r.ProfileName, r.Mode, string(seasons), r.Anime, r.SeasonCount, t, t)
	if err != nil {
		return Request{}, err
	}
	id, _ := res.LastInsertId()
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
		WHERE r.type = ? AND r.tmdb_id = ? AND r.status IN ('pending', 'approved', 'sent', 'downloading', 'available') ORDER BY r.id DESC LIMIT 1`, typ, tmdbID))
	if errors.Is(err, sql.ErrNoRows) {
		return Request{}, false, nil
	}
	return r, err == nil, err
}

// ActiveRequests lists requests that went to a Radarr/Sonarr and aren't available yet (the status job follows them).
func (s *Store) ActiveRequests() ([]Request, error) {
	return s.queryRequests(`SELECT ` + requestCols + ` FROM requests r LEFT JOIN users u ON u.id = r.user_id WHERE r.status IN ('sent', 'downloading') AND r.arr_id != 0`)
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
	rows, err := s.db.Query(`SELECT ` + requestCols + ` FROM requests r LEFT JOIN users u ON u.id = r.user_id WHERE r.status IN ('pending', 'approved', 'sent', 'downloading', 'available') ORDER BY r.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	want := map[string]bool{}
	for _, k := range keys {
		want[k] = true
	}
	for rows.Next() {
		r, err := scanRequest(rows)
		if err != nil {
			return nil, err
		}
		k := r.Type + ":" + itoa(r.TmdbID)
		if want[k] {
			out[k] = r
		}
	}
	return out, rows.Err()
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

// UpdateRequest saves a request's progress fields.
func (s *Store) UpdateRequest(r Request) error {
	var server any
	if r.ServerID != 0 {
		server = r.ServerID
	}
	_, err := s.db.Exec(`UPDATE requests SET status = ?, server_id = ?, arr_id = ?, profile_id = ?, profile_name = ?, progress = ?, error = ?, updated_at = ? WHERE id = ?`,
		r.Status, server, r.ArrID, r.ProfileID, r.ProfileName, r.Progress, r.Error, now(), r.ID)
	return err
}

// DecideRequest records who approved or declined a request.
func (s *Store) DecideRequest(id, userID int64, status string) error {
	_, err := s.db.Exec(`UPDATE requests SET status = ?, decided_by = ?, updated_at = ? WHERE id = ?`, status, userID, now(), id)
	return err
}

// DeleteRequest removes a request.
func (s *Store) DeleteRequest(id int64) error {
	_, err := s.db.Exec(`DELETE FROM requests WHERE id = ?`, id)
	return err
}
