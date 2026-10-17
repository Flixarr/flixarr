package store

import (
	"encoding/json"
	"fmt"
)

// Rows brought over from Seerr (Settings → Import) keep their original dates and send nothing: no
// notifications, no alerts. Each remembers its Seerr ID so importing again skips it.

// SeerrIDs maps Seerr IDs to Flixarr IDs for what was already imported into "users", "requests" or "issues".
func (s *Store) SeerrIDs(table string) (map[int64]int64, error) {
	switch table {
	case "users", "requests", "issues":
	default:
		return nil, fmt.Errorf("unknown table %q", table)
	}
	rows, err := s.db.Query(`SELECT seerr_id, id FROM ` + table + ` WHERE seerr_id IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]int64{}
	for rows.Next() {
		var sid, id int64
		if rows.Scan(&sid, &id) == nil {
			out[sid] = id
		}
	}
	return out, rows.Err()
}

// LinkSeerrUser marks an existing person as a Seerr user (nothing else about them changes).
func (s *Store) LinkSeerrUser(id, seerrID int64) error {
	_, err := s.db.Exec(`UPDATE users SET seerr_id = ? WHERE id = ? AND seerr_id IS NULL`, seerrID, id)
	return err
}

// LocalUserByName finds a local account by username (any case).
func (s *Store) LocalUserByName(name string) (*User, error) {
	return scanUser(s.db.QueryRow(`SELECT `+userCols+` FROM users WHERE plex_id = 0 AND lower(username) = lower(?)`, name))
}

// ImportUser adds a person from Seerr with their permissions, request limits and join date. Local
// accounts come without a password (an admin sets one in Users).
func (s *Store) ImportUser(u User, seerrID int64) (*User, error) {
	created := u.CreatedAt
	nu, err := s.CreateUser(u)
	if err != nil {
		return nil, err
	}
	if created == "" {
		created = nu.CreatedAt
	}
	if _, err := s.db.Exec(`UPDATE users SET seerr_id = ?, movie_limit = ?, season_limit = ?, created_at = ? WHERE id = ?`,
		seerrID, u.MovieLimit, u.SeasonLimit, created, nu.ID); err != nil {
		return nil, err
	}
	return scanUser(s.db.QueryRow(`SELECT `+userCols+` FROM users WHERE id = ?`, nu.ID))
}

// ImportRequest saves a request from Seerr with its own dates and timeline.
func (s *Store) ImportRequest(r Request, seerrID, decidedBy int64, events []Event) (int64, error) {
	if r.Seasons == nil {
		r.Seasons = []int{}
	}
	seasons, _ := json.Marshal(r.Seasons)
	var server, decider any
	if r.ServerID != 0 {
		server = r.ServerID
	}
	if decidedBy != 0 {
		decider = decidedBy
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`INSERT INTO requests (type, tmdb_id, title, year, poster, backdrop, user_id, status, server_id, arr_id, profile_id, profile_name, mode, seasons, anime, season_count, episodes, error, created_at, updated_at, source, seerr_id, decided_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '[]', ?, ?, ?, ?, ?, ?)`,
		r.Type, r.TmdbID, r.Title, r.Year, r.Poster, r.Backdrop, r.UserID, r.Status, server, r.ArrID, r.ProfileID, r.ProfileName, r.Mode, string(seasons), r.Anime, r.SeasonCount,
		r.Error, r.CreatedAt, r.UpdatedAt, r.Source, seerrID, decider)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	for _, e := range events {
		if _, err := tx.Exec(`INSERT INTO request_events (request_id, status, note, at) VALUES (?, ?, ?, ?)`, id, e.Status, e.Note, e.At); err != nil {
			return 0, err
		}
	}
	return id, tx.Commit()
}

// ImportIssue saves an issue from Seerr with its comments, all with their own dates.
func (s *Store) ImportIssue(i Issue, seerrID, resolvedBy int64, comments []Comment) (int64, error) {
	if i.Parts == nil {
		i.Parts = [][2]int{}
	}
	parts, _ := json.Marshal(i.Parts)
	var resolver any
	if i.Status == "resolved" && resolvedBy != 0 {
		resolver = resolvedBy
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`INSERT INTO issues (type, tmdb_id, title, year, poster, kind, sub, season, episode, note, user_id, status, resolved_by, created_at, updated_at, parts, seerr_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, i.Type, i.TmdbID, i.Title, i.Year, i.Poster, i.Kind, i.Sub, i.Season, i.Episode, i.Note, i.UserID,
		i.Status, resolver, i.CreatedAt, i.UpdatedAt, string(parts), seerrID)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	for _, c := range comments {
		if _, err := tx.Exec(`INSERT INTO issue_comments (issue_id, user_id, body, created_at) VALUES (?, ?, ?, ?)`, id, c.UserID, c.Body, c.CreatedAt); err != nil {
			return 0, err
		}
	}
	return id, tx.Commit()
}

// PlexKeys lists every title in Plex as "movie:603" / "series:1396".
func (s *Store) PlexKeys() (map[string]bool, error) {
	rows, err := s.db.Query(`SELECT type, tmdb_id FROM plex_items`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var typ string
		var id int
		if rows.Scan(&typ, &id) == nil {
			out[fmt.Sprintf("%s:%d", typ, id)] = true
		}
	}
	return out, rows.Err()
}
