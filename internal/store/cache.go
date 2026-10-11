package store

import (
	"database/sql"
	"errors"
	"time"
)

// CacheEntry is one saved answer from the Flixarr API.
type CacheEntry struct {
	Kind    string
	Status  int
	Body    []byte
	Expires time.Time
}

// CacheKind is the saved-result count and size for one kind (titles, people, lists, searches).
type CacheKind struct {
	Kind  string `json:"kind"`
	Count int    `json:"count"`
	Bytes int64  `json:"bytes"`
}

// CacheGet returns a saved answer, expired or not (the caller decides whether a stale one is usable).
func (s *Store) CacheGet(key string) (*CacheEntry, error) {
	e := &CacheEntry{}
	var exp int64
	err := s.db.QueryRow(`SELECT kind, status, body, expires_at FROM cache WHERE key = ?`, key).Scan(&e.Kind, &e.Status, &e.Body, &exp)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	e.Expires = time.Unix(exp, 0)
	return e, nil
}

// CachePut saves (or replaces) an answer.
func (s *Store) CachePut(key string, e CacheEntry) error {
	_, err := s.db.Exec(`INSERT INTO cache (key, kind, status, body, fetched_at, expires_at) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET kind = excluded.kind, status = excluded.status, body = excluded.body,
		fetched_at = excluded.fetched_at, expires_at = excluded.expires_at`,
		key, e.Kind, e.Status, e.Body, time.Now().Unix(), e.Expires.Unix())
	return err
}

// CacheStats returns the count and size of saved answers per kind.
func (s *Store) CacheStats() ([]CacheKind, error) {
	rows, err := s.db.Query(`SELECT kind, COUNT(*), COALESCE(SUM(LENGTH(body)), 0) FROM cache GROUP BY kind`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CacheKind
	for rows.Next() {
		var k CacheKind
		if err := rows.Scan(&k.Kind, &k.Count, &k.Bytes); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// CacheFlush deletes every saved answer of one kind, or all of them when kind is "".
func (s *Store) CacheFlush(kind string) (int64, error) {
	var res sql.Result
	var err error
	if kind == "" {
		res, err = s.db.Exec(`DELETE FROM cache`)
	} else {
		res, err = s.db.Exec(`DELETE FROM cache WHERE kind = ?`, kind)
	}
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// CacheDeleteExpired removes answers that expired before cutoff.
func (s *Store) CacheDeleteExpired(cutoff time.Time) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM cache WHERE expires_at < ?`, cutoff.Unix())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// CacheTrim keeps only the newest max answers.
func (s *Store) CacheTrim(max int) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM cache WHERE key IN (SELECT key FROM cache ORDER BY fetched_at DESC LIMIT -1 OFFSET ?)`, max)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
