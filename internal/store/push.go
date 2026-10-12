package store

// PushSub is one device's Web Push subscription.
type PushSub struct {
	Endpoint string
	UserID   int64
	P256dh   string
	Auth     string
	Device   string
}

// SavePushSub adds (or moves to this user) a device's subscription.
func (s *Store) SavePushSub(p PushSub) error {
	_, err := s.db.Exec(`INSERT INTO push_subs (endpoint, user_id, p256dh, auth, device, created_at) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(endpoint) DO UPDATE SET user_id = excluded.user_id, p256dh = excluded.p256dh, auth = excluded.auth, device = excluded.device`,
		p.Endpoint, p.UserID, p.P256dh, p.Auth, p.Device, now())
	return err
}

// DeletePushSub forgets a subscription (userID 0 = whoever it belongs to).
func (s *Store) DeletePushSub(endpoint string, userID int64) error {
	_, err := s.db.Exec(`DELETE FROM push_subs WHERE endpoint = ? AND (? = 0 OR user_id = ?)`, endpoint, userID, userID)
	return err
}

// PushSubs lists a user's subscriptions (userID 0 = everyone's).
func (s *Store) PushSubs(userID int64) ([]PushSub, error) {
	rows, err := s.db.Query(`SELECT endpoint, user_id, p256dh, auth, device FROM push_subs WHERE ? = 0 OR user_id = ?`, userID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PushSub{}
	for rows.Next() {
		var p PushSub
		if err := rows.Scan(&p.Endpoint, &p.UserID, &p.P256dh, &p.Auth, &p.Device); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
