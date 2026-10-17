package store

import (
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"
)

// Alerts are the bell (#90). One alert per thing that happens. News (Ready to watch, Declined…) stays
// until it's tapped and for 24 hours after; a to-do (waiting for approval, a new issue, a server problem)
// stays until it's handled, tapped or not. Red = new since the person last opened the bell; the number =
// alerts they haven't tapped. alert_seen holds who tapped what.
type Alert struct {
	ID     int64  `json:"id"`
	Key    string `json:"key"`
	Kind   string `json:"kind"` // news | todo
	Title  string `json:"title"`
	Detail string `json:"detail"`
	Link   string `json:"link"`
	Poster string `json:"poster"`
	At     string `json:"at"`
	Tapped bool   `json:"tapped"`
	New    bool   `json:"new"`

	Audience string `json:"-"` // user:<id>, admins, everyone, or can:<permission>
	Repeat   bool   `json:"-"` // AddNews: the key may happen again (Ready to watch again), so always add
}

// tappedNewsFor is how long tapped news stays in the bell.
const tappedNewsFor = 24 * time.Hour

// bellSize is how many alerts a person's bell lists.
const bellSize = 50

// reopenWithin: a to-do (a server problem) that clears and comes back this soon is the same alert again.
const reopenWithin = 30 * time.Minute

// AddNews adds a news alert. With a Key, it's added only once (an update announcement, say).
func (s *Store) AddNews(a Alert) error {
	if a.Key != "" && !a.Repeat {
		var n int
		s.db.QueryRow(`SELECT COUNT(*) FROM alerts WHERE key = ?`, a.Key).Scan(&n)
		if n > 0 {
			return nil
		}
	}
	return s.insertAlert("news", a)
}

func (s *Store) insertAlert(kind string, a Alert) error {
	t := now()
	if _, err := s.db.Exec(`INSERT INTO alerts (key, audience, severity, title, detail, link, poster, kind, started_at, changed_at) VALUES (?, ?, 'info', ?, ?, ?, ?, ?, ?, ?)`,
		a.Key, a.Audience, a.Title, a.Detail, a.Link, a.Poster, kind, t, t); err != nil {
		return err
	}
	s.alertChanged(a.Audience)
	return nil
}

// RaiseTodo adds the to-do for a key, or updates its words quietly if it's already there (it doesn't
// turn new again). Returns the alert's ID.
func (s *Store) RaiseTodo(a Alert) (int64, error) {
	var id int64
	err := s.db.QueryRow(`SELECT id FROM alerts WHERE key = ? AND kind = 'todo' AND resolved_at IS NULL`, a.Key).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		// Back soon after it was handled (a server problem flapping): the same alert again.
		err = s.db.QueryRow(`SELECT id FROM alerts WHERE key = ? AND kind = 'todo' AND resolved_at > ? ORDER BY id DESC LIMIT 1`,
			a.Key, time.Now().Add(-reopenWithin).UTC().Format(time.RFC3339)).Scan(&id)
		if err == nil {
			_, err = s.db.Exec(`UPDATE alerts SET resolved_at = NULL WHERE id = ?`, id)
		}
	}
	if errors.Is(err, sql.ErrNoRows) {
		if err := s.insertAlert("todo", a); err != nil {
			return 0, err
		}
		s.db.QueryRow(`SELECT id FROM alerts WHERE key = ? AND kind = 'todo' AND resolved_at IS NULL`, a.Key).Scan(&id)
		return id, nil
	}
	if err != nil {
		return 0, err
	}
	_, err = s.db.Exec(`UPDATE alerts SET title = ?, detail = ?, link = ?, poster = ?, changed_at = ? WHERE id = ?`, a.Title, a.Detail, a.Link, a.Poster, now(), id)
	s.alertChanged(a.Audience)
	return id, err
}

// ResolveTodo: the to-do for a key was handled; it leaves everyone's bell.
func (s *Store) ResolveTodo(key string) error {
	var audience string
	if s.db.QueryRow(`SELECT audience FROM alerts WHERE key = ? AND kind = 'todo' AND resolved_at IS NULL`, key).Scan(&audience) != nil {
		return nil // nothing open
	}
	_, err := s.db.Exec(`UPDATE alerts SET resolved_at = ? WHERE key = ? AND kind = 'todo' AND resolved_at IS NULL`, now(), key)
	if err == nil {
		s.alertChanged(audience)
	}
	return err
}

// OpenTodoKeys lists the keys of open to-dos starting with a prefix.
func (s *Store) OpenTodoKeys(prefix string) []string {
	keys := []string{}
	rows, err := s.db.Query(`SELECT key FROM alerts WHERE kind = 'todo' AND resolved_at IS NULL AND key LIKE ? || '%'`, prefix)
	if err != nil {
		return keys
	}
	defer rows.Close()
	for rows.Next() {
		var k string
		if rows.Scan(&k) == nil {
			keys = append(keys, k)
		}
	}
	return keys
}

func (s *Store) alertChanged(audience string) {
	if s.OnAlert != nil {
		s.OnAlert(audience)
	}
}

// CanAll and CanNone are permission checks for callers without a user (tests, internal cleanups).
var (
	CanAll  = func(string) bool { return true }
	CanNone = func(string) bool { return false }
)

// SeenBy reports whether an alert's audience includes a user: "everyone", "admins", "user:<id>", or
// "can:<permission>" (everyone with that permission; admins have them all).
func SeenBy(audience string, userID int64, can func(string) bool) bool {
	switch {
	case audience == "everyone":
		return true
	case audience == "admins":
		return can("admin")
	case strings.HasPrefix(audience, "can:"):
		return can(strings.TrimPrefix(audience, "can:"))
	}
	return audience == "user:"+strconv.FormatInt(userID, 10)
}

// Bell lists a person's alerts, newest first: open to-dos, untapped news, and news tapped within the last
// day (can = their permission check). New = arrived since they last opened the bell, not tapped.
func (s *Store) Bell(userID int64, can func(string) bool) ([]Alert, error) {
	var opened string
	s.db.QueryRow(`SELECT bell_opened_at FROM users WHERE id = ?`, userID).Scan(&opened)
	since := time.Now().Add(-tappedNewsFor).UTC().Format(time.RFC3339)
	rows, err := s.db.Query(`SELECT a.id, a.key, a.kind, a.title, a.detail, a.link, a.poster, a.started_at, a.audience, t.seen_at IS NOT NULL
		FROM alerts a LEFT JOIN alert_seen t ON t.alert_id = a.id AND t.user_id = ?
		WHERE (a.kind = 'todo' AND a.resolved_at IS NULL) OR (a.kind = 'news' AND (t.seen_at IS NULL OR t.seen_at > ?))
		ORDER BY a.id DESC LIMIT 2000`, userID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Alert{}
	for rows.Next() && len(out) < bellSize {
		var a Alert
		var audience string
		if err := rows.Scan(&a.ID, &a.Key, &a.Kind, &a.Title, &a.Detail, &a.Link, &a.Poster, &a.At, &audience, &a.Tapped); err != nil {
			return nil, err
		}
		if !SeenBy(audience, userID, can) {
			continue
		}
		a.New = !a.Tapped && a.At > opened
		out = append(out, a)
	}
	return out, rows.Err()
}

// OpenedBell: the person opened the bell; what's there isn't new any more (still untapped).
func (s *Store) OpenedBell(userID int64) error {
	_, err := s.db.Exec(`UPDATE users SET bell_opened_at = ? WHERE id = ?`, now(), userID)
	return err
}

// TapAlert marks one alert tapped by a person.
func (s *Store) TapAlert(userID, alertID int64) error {
	_, err := s.db.Exec(`INSERT INTO alert_seen (user_id, alert_id, seen_at) VALUES (?, ?, ?) ON CONFLICT(user_id, alert_id) DO NOTHING`, userID, alertID, now())
	return err
}

// TapAlertKey marks a key's newest alert tapped by a person: the one who caused it already knows.
func (s *Store) TapAlertKey(userID int64, key string) error {
	_, err := s.db.Exec(`INSERT INTO alert_seen (user_id, alert_id, seen_at) SELECT ?, id, ? FROM alerts WHERE key = ? ORDER BY id DESC LIMIT 1
		ON CONFLICT(user_id, alert_id) DO NOTHING`, userID, now(), key)
	return err
}

// TapAll marks everything in a person's bell tapped (to-dos stay until handled).
func (s *Store) TapAll(userID int64, can func(string) bool) error {
	list, err := s.Bell(userID, can)
	if err != nil {
		return err
	}
	for _, a := range list {
		if !a.Tapped {
			s.TapAlert(userID, a.ID)
		}
	}
	return nil
}

// LastAlertAt is when the newest alert with a key was added (ok=false: never).
func (s *Store) LastAlertAt(key string) (string, bool) {
	var at string
	err := s.db.QueryRow(`SELECT started_at FROM alerts WHERE key = ? ORDER BY id DESC LIMIT 1`, key).Scan(&at)
	return at, err == nil
}
