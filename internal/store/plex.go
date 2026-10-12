package store

import "strings"

// PlexItem is a title in Plex.
type PlexItem struct {
	Type      string
	TmdbID    int
	RatingKey string
	Section   string
	Title     string
	AddedAt   int64
	SeenAt    string
}

// SavePlexItem records a title seen in Plex; isNew says it wasn't known before.
func (s *Store) SavePlexItem(it PlexItem) (isNew bool, err error) {
	var n int
	s.db.QueryRow(`SELECT COUNT(*) FROM plex_items WHERE type = ? AND tmdb_id = ?`, it.Type, it.TmdbID).Scan(&n)
	_, err = s.db.Exec(`INSERT INTO plex_items (type, tmdb_id, rating_key, section, title, added_at, seen_at) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(type, tmdb_id) DO UPDATE SET rating_key = excluded.rating_key, section = excluded.section, title = excluded.title, added_at = excluded.added_at, seen_at = excluded.seen_at`,
		it.Type, it.TmdbID, it.RatingKey, it.Section, it.Title, it.AddedAt, it.SeenAt)
	return n == 0 && err == nil, err
}

// PrunePlexItems drops titles no scan has seen since the given time.
func (s *Store) PrunePlexItems(since string) (int, error) {
	res, err := s.db.Exec(`DELETE FROM plex_items WHERE seen_at < ?`, since)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
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
		err := s.db.QueryRow(`SELECT type, tmdb_id, rating_key, section, title, added_at, seen_at FROM plex_items WHERE type = ? AND tmdb_id = ?`, typ, id).
			Scan(&it.Type, &it.TmdbID, &it.RatingKey, &it.Section, &it.Title, &it.AddedAt, &it.SeenAt)
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
