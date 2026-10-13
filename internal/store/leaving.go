package store

import (
	"database/sql"
	"errors"
	"time"
)

// Leaving is a title in one of Maintainerr's collections: it's deleted at LeaveAt unless kept.
type Leaving struct {
	Type         string `json:"type"` // movie | series
	TmdbID       int    `json:"tmdbId"`
	PlexID       string `json:"plexId"`
	CollectionID int    `json:"collectionId"`
	Collection   string `json:"collection"`
	LeaveAt      string `json:"leaveAt"` // RFC3339
	SeenAt       string `json:"-"`
}

// goneFor is how long a title is remembered after it left the collections (so a removal soon after can be
// put down to Maintainerr).
const goneFor = 30 * 24 * time.Hour

// SyncLeaving records what's in Maintainerr's collections now (seenAt = this read), and forgets titles
// that left them long ago.
func (s *Store) SyncLeaving(items []Leaving, seenAt string) error {
	for _, it := range items {
		if _, err := s.db.Exec(`INSERT INTO leaving (type, tmdb_id, plex_id, collection_id, collection, leave_at, seen_at) VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(type, tmdb_id) DO UPDATE SET plex_id = excluded.plex_id, collection_id = excluded.collection_id, collection = excluded.collection,
			leave_at = excluded.leave_at, seen_at = excluded.seen_at`, it.Type, it.TmdbID, it.PlexID, it.CollectionID, it.Collection, it.LeaveAt, seenAt); err != nil {
			return err
		}
	}
	_, err := s.db.Exec(`DELETE FROM leaving WHERE seen_at < ?`, time.Now().Add(-goneFor).UTC().Format(time.RFC3339))
	return err
}

// LeavingNow lists the titles in the collections as of the read at seenAt, soonest first.
func (s *Store) LeavingNow(seenAt string) ([]Leaving, error) {
	return s.queryLeaving(`SELECT type, tmdb_id, plex_id, collection_id, collection, leave_at, seen_at FROM leaving WHERE seen_at = ? ORDER BY leave_at`, seenAt)
}

// LeavingFor looks titles up ("movie:603" keys) among those in the collections as of seenAt.
func (s *Store) LeavingFor(keys []string, seenAt string) (map[string]Leaving, error) {
	all, err := s.LeavingNow(seenAt)
	if err != nil {
		return nil, err
	}
	want := map[string]bool{}
	for _, k := range keys {
		want[k] = true
	}
	out := map[string]Leaving{}
	for _, l := range all {
		if k := l.Type + ":" + itoa(l.TmdbID); want[k] {
			out[k] = l
		}
	}
	return out, nil
}

// WasLeaving says whether a title was in a Maintainerr collection lately (it may have been deleted since).
func (s *Store) WasLeaving(typ string, tmdbID int) (Leaving, bool) {
	list, _ := s.queryLeaving(`SELECT type, tmdb_id, plex_id, collection_id, collection, leave_at, seen_at FROM leaving WHERE type = ? AND tmdb_id = ?`, typ, tmdbID)
	if len(list) == 0 {
		return Leaving{}, false
	}
	return list[0], true
}

func (s *Store) queryLeaving(q string, args ...any) ([]Leaving, error) {
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Leaving{}
	for rows.Next() {
		var l Leaving
		if err := rows.Scan(&l.Type, &l.TmdbID, &l.PlexID, &l.CollectionID, &l.Collection, &l.LeaveAt, &l.SeenAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// Keep is a title Flixarr kept out of Maintainerr's rules.
type Keep struct {
	Type    string `json:"type"`
	TmdbID  int    `json:"tmdbId"`
	PlexID  string `json:"plexId"`
	Reason  string `json:"reason"` // kept | fresh
	UntilAt string `json:"untilAt"`
	ByUser  int64  `json:"byUser"`
}

// SaveKeep records a keep (replacing a fresh one with a lasting one, never the other way round).
func (s *Store) SaveKeep(k Keep) error {
	_, err := s.db.Exec(`INSERT INTO keeps (type, tmdb_id, plex_id, reason, until_at, by_user, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(type, tmdb_id) DO UPDATE SET plex_id = excluded.plex_id, reason = excluded.reason, until_at = excluded.until_at, by_user = excluded.by_user
		WHERE keeps.reason = 'fresh'`, k.Type, k.TmdbID, k.PlexID, k.Reason, k.UntilAt, k.ByUser, now())
	return err
}

// KeepFor returns a title's keep, if any.
func (s *Store) KeepFor(typ string, tmdbID int) (Keep, bool) {
	var k Keep
	err := s.db.QueryRow(`SELECT type, tmdb_id, plex_id, reason, until_at, by_user FROM keeps WHERE type = ? AND tmdb_id = ?`, typ, tmdbID).
		Scan(&k.Type, &k.TmdbID, &k.PlexID, &k.Reason, &k.UntilAt, &k.ByUser)
	return k, err == nil
}

// Keeps lists every keep.
func (s *Store) Keeps() ([]Keep, error) {
	rows, err := s.db.Query(`SELECT type, tmdb_id, plex_id, reason, until_at, by_user FROM keeps`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Keep{}
	for rows.Next() {
		var k Keep
		if err := rows.Scan(&k.Type, &k.TmdbID, &k.PlexID, &k.Reason, &k.UntilAt, &k.ByUser); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// DropKeep forgets a keep.
func (s *Store) DropKeep(typ string, tmdbID int) error {
	_, err := s.db.Exec(`DELETE FROM keeps WHERE type = ? AND tmdb_id = ?`, typ, tmdbID)
	return err
}

// AskToKeep records that a person wants a leaving title kept.
func (s *Store) AskToKeep(typ string, tmdbID int, userID int64) error {
	_, err := s.db.Exec(`INSERT OR IGNORE INTO keep_asks (type, tmdb_id, user_id, created_at) VALUES (?, ?, ?, ?)`, typ, tmdbID, userID, now())
	return err
}

// KeepAsk is a request to keep a title.
type KeepAsk struct {
	Type     string `json:"type"`
	TmdbID   int    `json:"tmdbId"`
	UserID   int64  `json:"userId"`
	UserName string `json:"userName"`
}

// KeepAsks lists who asked to keep what.
func (s *Store) KeepAsks() ([]KeepAsk, error) {
	rows, err := s.db.Query(`SELECT k.type, k.tmdb_id, k.user_id, COALESCE(u.username, '') FROM keep_asks k LEFT JOIN users u ON u.id = k.user_id ORDER BY k.created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []KeepAsk{}
	for rows.Next() {
		var a KeepAsk
		if err := rows.Scan(&a.Type, &a.TmdbID, &a.UserID, &a.UserName); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ClearKeepAsks forgets the asks for a title (it was kept, given more time, or let go).
func (s *Store) ClearKeepAsks(typ string, tmdbID int) error {
	_, err := s.db.Exec(`DELETE FROM keep_asks WHERE type = ? AND tmdb_id = ?`, typ, tmdbID)
	return err
}

// ToldLeaving says whether a request's person was already told about this leave date, and remembers it.
func (s *Store) ToldLeaving(requestID int64, leaveAt string) (already bool) {
	var at string
	err := s.db.QueryRow(`SELECT leave_at FROM leaving_told WHERE request_id = ?`, requestID).Scan(&at)
	if err == nil && at == leaveAt {
		return true
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return true // can't tell: don't repeat
	}
	s.db.Exec(`INSERT INTO leaving_told (request_id, leave_at) VALUES (?, ?) ON CONFLICT(request_id) DO UPDATE SET leave_at = excluded.leave_at`, requestID, leaveAt)
	return false
}

// AvailableRequestsSince lists available requests last changed after since (for fresh keeps).
func (s *Store) AvailableRequestsSince(since string) ([]Request, error) {
	return s.queryRequests(`SELECT `+requestCols+` FROM requests r LEFT JOIN users u ON u.id = r.user_id WHERE r.status = 'available' AND r.updated_at > ?`, since)
}
