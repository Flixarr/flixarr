package store

import (
	"strings"
	"time"
)

// PlexItem is a title in Plex.
type PlexItem struct {
	Type      string
	TmdbID    int
	RatingKey string
	Section   string
	Title     string
	AddedAt   int64
	SeenAt    string
	// Series: episodes Plex has of those aired so far (#137); EpsTotal 0 = not checked yet.
	EpsHave, EpsTotal int
}

// SavePlexItem records a title seen in Plex; isNew says it wasn't known before.
func (s *Store) SavePlexItem(it PlexItem) (isNew bool, err error) {
	var n int
	s.db.QueryRow(`SELECT COUNT(*) FROM plex_items WHERE type = ? AND tmdb_id = ?`, it.Type, it.TmdbID).Scan(&n)
	_, err = s.db.Exec(`INSERT INTO plex_items (type, tmdb_id, rating_key, section, title, added_at, seen_at) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(type, tmdb_id) DO UPDATE SET rating_key = excluded.rating_key, section = excluded.section, title = excluded.title, added_at = excluded.added_at, seen_at = MAX(seen_at, excluded.seen_at)`,
		it.Type, it.TmdbID, it.RatingKey, it.Section, it.Title, it.AddedAt, it.SeenAt)
	if n == 0 && err == nil {
		at := ""
		if it.AddedAt > 0 {
			at = time.Unix(it.AddedAt, 0).UTC().Format(time.RFC3339)
		}
		s.AddTitleEvent(it.Type, it.TmdbID, "plex", "", 0, at) // History: when it arrived in Plex
	}
	return n == 0 && err == nil, err
}

// PrunePlexItems drops titles no scan has seen since the given time and returns their keys ("movie:603").
func (s *Store) PrunePlexItems(since string) ([]string, error) {
	return s.dropPlexItems(`seen_at < ?`, since)
}

// RemovePlexItems drops the titles Plex knows by these rating keys and returns their keys.
func (s *Store) RemovePlexItems(ratingKeys []string) ([]string, error) {
	out := []string{}
	for _, k := range ratingKeys {
		got, err := s.dropPlexItems(`rating_key = ?`, k)
		if err != nil {
			return out, err
		}
		out = append(out, got...)
	}
	return out, nil
}

func (s *Store) dropPlexItems(where string, arg any) ([]string, error) {
	rows, err := s.db.Query(`SELECT type, tmdb_id FROM plex_items WHERE `+where, arg)
	if err != nil {
		return nil, err
	}
	keys := []string{}
	for rows.Next() {
		var typ string
		var id int
		if err := rows.Scan(&typ, &id); err != nil {
			rows.Close()
			return nil, err
		}
		keys = append(keys, typ+":"+itoa(id))
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return keys, nil
	}
	_, err = s.db.Exec(`DELETE FROM plex_items WHERE `+where, arg)
	return keys, err
}

// ClearPlexItems forgets everything from Plex.
func (s *Store) ClearPlexItems() error {
	_, err := s.db.Exec(`DELETE FROM plex_items`)
	return err
}

// PlexItemCounts counts titles per library section.
func (s *Store) PlexItemCounts() (map[string]int, error) {
	rows, err := s.db.Query(`SELECT section, COUNT(*) FROM plex_items GROUP BY section`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var k string
		var n int
		if err := rows.Scan(&k, &n); err != nil {
			return nil, err
		}
		out[k] = n
	}
	return out, rows.Err()
}

// PlexItemsFor looks up titles by "movie:603" / "series:1396" keys.
func (s *Store) PlexItemsFor(keys []string) (map[string]PlexItem, error) {
	out := map[string]PlexItem{}
	for _, k := range keys {
		typ, id, ok := strings.Cut(k, ":")
		if !ok {
			continue
		}
		var it PlexItem
		err := s.db.QueryRow(`SELECT type, tmdb_id, rating_key, section, title, added_at, seen_at, eps_have, eps_total FROM plex_items WHERE type = ? AND tmdb_id = ?`, typ, id).
			Scan(&it.Type, &it.TmdbID, &it.RatingKey, &it.Section, &it.Title, &it.AddedAt, &it.SeenAt, &it.EpsHave, &it.EpsTotal)
		if err == nil {
			out[k] = it
		}
	}
	return out, nil
}

// RecentPlexItems lists the newest titles in Plex.
func (s *Store) RecentPlexItems(limit int) ([]PlexItem, error) {
	rows, err := s.db.Query(`SELECT type, tmdb_id, rating_key, section, title, added_at, seen_at FROM plex_items ORDER BY added_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PlexItem{}
	for rows.Next() {
		var it PlexItem
		if err := rows.Scan(&it.Type, &it.TmdbID, &it.RatingKey, &it.Section, &it.Title, &it.AddedAt, &it.SeenAt); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// PlexItemsPage lists one page of a type's titles in Plex: newest first, or by title.
func (s *Store) PlexItemsPage(typ, sort string, page, size int) ([]PlexItem, int, error) {
	var total int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM plex_items WHERE type = ?`, typ).Scan(&total); err != nil {
		return nil, 0, err
	}
	order := "added_at DESC"
	if sort == "title" {
		order = "title COLLATE NOCASE"
	}
	rows, err := s.db.Query(`SELECT type, tmdb_id, rating_key, section, title, added_at, seen_at FROM plex_items WHERE type = ? ORDER BY `+order+` LIMIT ? OFFSET ?`, typ, size, (page-1)*size)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []PlexItem{}
	for rows.Next() {
		var it PlexItem
		if err := rows.Scan(&it.Type, &it.TmdbID, &it.RatingKey, &it.Section, &it.Title, &it.AddedAt, &it.SeenAt); err != nil {
			return nil, 0, err
		}
		out = append(out, it)
	}
	return out, total, rows.Err()
}

// PlexItemByRatingKey finds a title by the key Plex knows it by.
func (s *Store) PlexItemByRatingKey(key string) (PlexItem, bool) {
	var it PlexItem
	err := s.db.QueryRow(`SELECT type, tmdb_id, rating_key, section, title, added_at, seen_at FROM plex_items WHERE rating_key = ?`, key).
		Scan(&it.Type, &it.TmdbID, &it.RatingKey, &it.Section, &it.Title, &it.AddedAt, &it.SeenAt)
	return it, err == nil
}

// PlexSeriesIDs lists the series in Plex (TMDB IDs).
func (s *Store) PlexSeriesIDs() ([]int, error) {
	rows, err := s.db.Query(`SELECT tmdb_id FROM plex_items WHERE type = 'series' ORDER BY tmdb_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []int{}
	for rows.Next() {
		var id int
		if rows.Scan(&id) == nil {
			out = append(out, id)
		}
	}
	return out, rows.Err()
}

// SetSeriesEpisodes records how much of a series Plex has (episodes it has of those aired so far),
// whether the show has ended, and when it was checked.
func (s *Store) SetSeriesEpisodes(tmdbID, have, total int, ended bool) error {
	_, err := s.db.Exec(`UPDATE plex_items SET eps_have = ?, eps_total = ?, eps_ended = ?, eps_checked = ? WHERE type = 'series' AND tmdb_id = ?`, have, total, ended, now(), tmdbID)
	return err
}

// SeriesCheckedLately lists the ended series checked within the last week (they can wait, #165).
func (s *Store) SeriesCheckedLately() (map[int]bool, error) {
	rows, err := s.db.Query(`SELECT tmdb_id FROM plex_items WHERE type = 'series' AND eps_ended = 1 AND eps_total > 0 AND eps_checked > ?`, time.Now().UTC().Add(-7*24*time.Hour).Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int]bool{}
	for rows.Next() {
		var id int
		if rows.Scan(&id) == nil {
			out[id] = true
		}
	}
	return out, rows.Err()
}
