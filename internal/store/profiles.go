package store

// ProfileLink is a Flixarr profile added to a server.
type ProfileLink struct {
	ServerID   int64  `json:"-"`
	Key        string `json:"key"`
	ArrID      int    `json:"arrId"`
	Rev        string `json:"rev"`
	PushedHash string `json:"-"`
	State      string `json:"state"`
	CheckedAt  string `json:"checkedAt"`
	UpdatedAt  string `json:"updatedAt"`
}

// FormatLink is a custom format Flixarr uses on a server.
type FormatLink struct {
	ServerID int64
	Name     string
	ArrID    int
	Owned    bool
	SpecHash string
}

// ProfileLinks lists the Flixarr profiles on a server.
func (s *Store) ProfileLinks(serverID int64) ([]ProfileLink, error) {
	rows, err := s.db.Query(`SELECT server_id, key, arr_id, rev, pushed_hash, state, checked_at, updated_at FROM profile_links WHERE server_id = ? ORDER BY key`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ProfileLink{}
	for rows.Next() {
		var l ProfileLink
		if err := rows.Scan(&l.ServerID, &l.Key, &l.ArrID, &l.Rev, &l.PushedHash, &l.State, &l.CheckedAt, &l.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// SaveProfileLink inserts or replaces a link. updated=true also stamps updated_at (a push happened).
func (s *Store) SaveProfileLink(l ProfileLink, updated bool) error {
	t := now()
	if l.UpdatedAt == "" || updated {
		l.UpdatedAt = t
	}
	_, err := s.db.Exec(`INSERT INTO profile_links (server_id, key, arr_id, rev, pushed_hash, state, checked_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(server_id, key) DO UPDATE SET arr_id = excluded.arr_id, rev = excluded.rev, pushed_hash = excluded.pushed_hash, state = excluded.state, checked_at = excluded.checked_at, updated_at = excluded.updated_at`,
		l.ServerID, l.Key, l.ArrID, l.Rev, l.PushedHash, l.State, t, l.UpdatedAt)
	return err
}

// DeleteProfileLink forgets a profile on a server.
func (s *Store) DeleteProfileLink(serverID int64, key string) error {
	_, err := s.db.Exec(`DELETE FROM profile_links WHERE server_id = ? AND key = ?`, serverID, key)
	return err
}

// FormatLinks lists the custom formats Flixarr uses on a server, by name.
func (s *Store) FormatLinks(serverID int64) (map[string]FormatLink, error) {
	rows, err := s.db.Query(`SELECT server_id, name, arr_id, owned, spec_hash FROM format_links WHERE server_id = ?`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]FormatLink{}
	for rows.Next() {
		var f FormatLink
		if err := rows.Scan(&f.ServerID, &f.Name, &f.ArrID, &f.Owned, &f.SpecHash); err != nil {
			return nil, err
		}
		out[f.Name] = f
	}
	return out, rows.Err()
}

// SaveFormatLink inserts or replaces a format link.
func (s *Store) SaveFormatLink(f FormatLink) error {
	_, err := s.db.Exec(`INSERT INTO format_links (server_id, name, arr_id, owned, spec_hash) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(server_id, name) DO UPDATE SET arr_id = excluded.arr_id, owned = excluded.owned, spec_hash = excluded.spec_hash`,
		f.ServerID, f.Name, f.ArrID, f.Owned, f.SpecHash)
	return err
}

// DeleteFormatLink forgets a format link.
func (s *Store) DeleteFormatLink(serverID int64, name string) error {
	_, err := s.db.Exec(`DELETE FROM format_links WHERE server_id = ? AND name = ?`, serverID, name)
	return err
}
