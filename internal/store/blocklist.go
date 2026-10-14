package store

import "strconv"

// Blocked is a title that can't be requested.
type Blocked struct {
	Type      string `json:"type"`
	TmdbID    int    `json:"tmdbId"`
	Title     string `json:"title"`
	Year      string `json:"year"`
	Poster    string `json:"poster"`
	UserName  string `json:"userName"`
	CreatedAt string `json:"createdAt"`
}

// Block adds a title to the blocklist.
func (s *Store) Block(b Blocked, by int64) error {
	_, err := s.db.Exec(`INSERT INTO blocklist (type, tmdb_id, title, year, poster, user_id, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(type, tmdb_id) DO NOTHING`, b.Type, b.TmdbID, b.Title, b.Year, b.Poster, by, now())
	return err
}

// Unblock removes a title from the blocklist.
func (s *Store) Unblock(typ string, tmdbID int) error {
	_, err := s.db.Exec(`DELETE FROM blocklist WHERE type = ? AND tmdb_id = ?`, typ, tmdbID)
	return err
}

// IsBlocked reports whether a title is on the blocklist.
func (s *Store) IsBlocked(typ string, tmdbID int) bool {
	var n int
	s.db.QueryRow(`SELECT COUNT(*) FROM blocklist WHERE type = ? AND tmdb_id = ?`, typ, tmdbID).Scan(&n)
	return n > 0
}

// BlockedKeys is every blocked title as a "movie:603" key.
func (s *Store) BlockedKeys() map[string]bool {
	out := map[string]bool{}
	rows, err := s.db.Query(`SELECT type, tmdb_id FROM blocklist`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var typ string
		var id int
		if rows.Scan(&typ, &id) == nil {
			out[typ+":"+strconv.Itoa(id)] = true
		}
	}
	return out
}

// Blocklist lists blocked titles, newest first.
func (s *Store) Blocklist() ([]Blocked, error) {
	rows, err := s.db.Query(`SELECT b.type, b.tmdb_id, b.title, b.year, b.poster, COALESCE(u.username, ''), b.created_at
		FROM blocklist b LEFT JOIN users u ON u.id = b.user_id ORDER BY b.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Blocked{}
	for rows.Next() {
		var b Blocked
		if err := rows.Scan(&b.Type, &b.TmdbID, &b.Title, &b.Year, &b.Poster, &b.UserName, &b.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
