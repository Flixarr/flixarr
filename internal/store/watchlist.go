package store

// SetPlexToken keeps a user's plex.tv token (encrypted), refreshed at each sign-in.
func (s *Store) SetPlexToken(userID int64, token string) error {
	_, err := s.db.Exec(`UPDATE users SET plex_token = ? WHERE id = ?`, s.box.Seal(token), userID)
	return err
}

// PlexToken returns a user's plex.tv token, or "" when there isn't one (signed in before it was kept).
func (s *Store) PlexToken(userID int64) string {
	var sealed string
	s.db.QueryRow(`SELECT plex_token FROM users WHERE id = ?`, userID).Scan(&sealed)
	if sealed == "" {
		return ""
	}
	v, err := s.box.Open(sealed)
	if err != nil {
		return ""
	}
	return v
}

// Watchlist reports whether a user has Plex Watchlist requests on.
func (s *Store) Watchlist(userID int64) bool {
	var on bool
	s.db.QueryRow(`SELECT watchlist FROM users WHERE id = ?`, userID).Scan(&on)
	return on
}

// SetWatchlist turns a user's Plex Watchlist requests on or off.
func (s *Store) SetWatchlist(userID int64, on bool) error {
	_, err := s.db.Exec(`UPDATE users SET watchlist = ? WHERE id = ?`, on, userID)
	return err
}

// WatchlistUsers lists the users with Plex Watchlist requests on.
func (s *Store) WatchlistUsers() ([]*User, error) {
	rows, err := s.db.Query(`SELECT ` + userCols + ` FROM users WHERE watchlist = 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// WatchlistSeen reports whether a watchlist title was already handled for a user.
func (s *Store) WatchlistSeen(userID int64, typ string, tmdbID int) bool {
	var n int
	s.db.QueryRow(`SELECT COUNT(*) FROM watchlist_seen WHERE user_id = ? AND type = ? AND tmdb_id = ?`, userID, typ, tmdbID).Scan(&n)
	return n > 0
}

// MarkWatchlistSeen records what happened to a watchlist title.
func (s *Store) MarkWatchlistSeen(userID int64, typ string, tmdbID int, result string) error {
	_, err := s.db.Exec(`INSERT INTO watchlist_seen (user_id, type, tmdb_id, result, seen_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(user_id, type, tmdb_id) DO UPDATE SET result = excluded.result, seen_at = excluded.seen_at`, userID, typ, tmdbID, result, now())
	return err
}
