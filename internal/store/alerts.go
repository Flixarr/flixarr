package store

import (
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"
)

// Alert is one entry in the bell. It turns unread again only when its Sig gets "louder" (see Louder):
// something new needs attention. Anything quieter (a count going down, a problem clearing while others
// stay, new wording) updates it in place without turning the bell red.
type Alert struct {
	ID        int64  `json:"id"`
	Key       string `json:"key"`
	Audience  string `json:"-"`        // everyone, admins, or user:<id>
	Severity  string `json:"severity"` // info, warning or error
	Title     string `json:"title"`
	Detail    string `json:"detail"`
	Link      string `json:"link"`
	Sig       string `json:"-"`
	StartedAt string `json:"startedAt"`
	ChangedAt string `json:"changedAt"`
	Unread    bool   `json:"unread"`
}

// reopenWithin: a problem that clears and comes back this soon reopens its old alert (keeping whether it
// was seen) instead of starting a new, unread one.
const reopenWithin = 30 * time.Minute

// RaiseAlert creates the active alert for a key, or updates it.
func (s *Store) RaiseAlert(a Alert) error {
	if a.Audience == "" {
		a.Audience = "admins"
	}
	t := now()
	var id int64
	var sig string
	err := s.db.QueryRow(`SELECT id, sig FROM alerts WHERE key = ? AND resolved_at IS NULL`, a.Key).Scan(&id, &sig)
	if errors.Is(err, sql.ErrNoRows) {
		// Back soon after it cleared: the same alert again.
		err = s.db.QueryRow(`SELECT id, sig FROM alerts WHERE key = ? AND resolved_at > ? ORDER BY id DESC LIMIT 1`,
			a.Key, time.Now().Add(-reopenWithin).UTC().Format(time.RFC3339)).Scan(&id, &sig)
		if err == nil {
			_, err = s.db.Exec(`UPDATE alerts SET resolved_at = NULL WHERE id = ?`, id)
		}
	}
	if errors.Is(err, sql.ErrNoRows) {
		_, err = s.db.Exec(`INSERT INTO alerts (key, audience, severity, title, detail, link, sig, started_at, changed_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			a.Key, a.Audience, a.Severity, a.Title, a.Detail, a.Link, a.Sig, t, t)
		s.alertChanged(a.Audience)
		return err
	}
	if err != nil {
		return err
	}
	if Louder(sig, a.Sig) {
		_, err = s.db.Exec(`UPDATE alerts SET severity = ?, title = ?, detail = ?, link = ?, sig = ?, changed_at = ? WHERE id = ?`, a.Severity, a.Title, a.Detail, a.Link, a.Sig, t, id)
	} else {
		// Quieter or the same: new wording and sig, seen state kept.
		_, err = s.db.Exec(`UPDATE alerts SET severity = ?, title = ?, detail = ?, link = ?, sig = ? WHERE id = ?`, a.Severity, a.Title, a.Detail, a.Link, a.Sig, id)
	}
	if err == nil {
		s.alertChanged(a.Audience) // reopened or reworded: browsers redraw (unread only if louder)
	}
	return err
}

// Louder says whether an alert's new sig needs attention again. Numbers (a waiting count): only a bigger
// one. Otherwise sigs are comma lists of what's wrong: louder when the new one names something the old didn't.
func Louder(old, new string) bool {
	if old == new {
		return false
	}
	if o, err1 := strconv.Atoi(old); err1 == nil {
		if n, err2 := strconv.Atoi(new); err2 == nil {
			return n > o
		}
	}
	had := map[string]bool{}
	for _, k := range strings.Split(old, ",") {
		had[k] = true
	}
	for _, k := range strings.Split(new, ",") {
		if k != "" && !had[k] {
			return true
		}
	}
	return false
}

func (s *Store) alertChanged(audience string) {
	if s.OnAlert != nil {
		s.OnAlert(audience)
	}
}

// ResolveAlert closes the active alert for a key, if there is one.
func (s *Store) ResolveAlert(key string) error {
	var audience string
	if s.db.QueryRow(`SELECT audience FROM alerts WHERE key = ? AND resolved_at IS NULL`, key).Scan(&audience) != nil {
		return nil // nothing open
	}
	_, err := s.db.Exec(`UPDATE alerts SET resolved_at = ? WHERE key = ? AND resolved_at IS NULL`, now(), key)
	if err == nil {
		s.alertChanged(audience)
	}
	return err
}

// ActiveAlerts lists the open alerts a user can see, newest change first, with their unread state.
func (s *Store) ActiveAlerts(userID int64, admin bool) ([]Alert, error) {
	rows, err := s.db.Query(`SELECT a.id, a.key, a.severity, a.title, a.detail, a.link, a.started_at, a.changed_at,
			(s.seen_at IS NULL OR s.seen_at < a.changed_at)
		FROM alerts a LEFT JOIN alert_seen s ON s.alert_id = a.id AND s.user_id = ?
		WHERE a.resolved_at IS NULL AND (a.audience = 'everyone' OR (a.audience = 'admins' AND ?) OR a.audience = 'user:' || ?)
		ORDER BY a.changed_at DESC`, userID, admin, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Alert{}
	for rows.Next() {
		var a Alert
		if err := rows.Scan(&a.ID, &a.Key, &a.Severity, &a.Title, &a.Detail, &a.Link, &a.StartedAt, &a.ChangedAt, &a.Unread); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// MarkAlertsSeen marks every open alert as seen by a user.
func (s *Store) MarkAlertsSeen(userID int64) error {
	_, err := s.db.Exec(`INSERT INTO alert_seen (user_id, alert_id, seen_at) SELECT ?, id, ? FROM alerts WHERE resolved_at IS NULL
		ON CONFLICT(user_id, alert_id) DO UPDATE SET seen_at = excluded.seen_at`, userID, now())
	return err
}
