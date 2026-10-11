package store

import (
	"database/sql"
	"errors"
)

// Server is a Radarr or Sonarr connection.
type Server struct {
	ID                 int64  `json:"id"`
	Kind               string `json:"kind"` // "radarr" or "sonarr"
	Name               string `json:"name"`
	URL                string `json:"url"`
	APIKey             string `json:"-"` // never sent to the browser
	RootFolder         string `json:"rootFolder"`
	QualityProfileID   int    `json:"qualityProfileId"`
	QualityProfileName string `json:"qualityProfileName"`
	SeasonFolders      bool   `json:"seasonFolders"`
	SearchOnAdd        bool   `json:"searchOnAdd"`
	Enabled            bool   `json:"enabled"`
	CreatedAt          string `json:"createdAt"`
	UpdatedAt          string `json:"updatedAt"`
}

const serverCols = `id, kind, name, url, api_key, root_folder, quality_profile_id, quality_profile_name, season_folders, search_on_add, enabled, created_at, updated_at`

func scanServer(row interface{ Scan(...any) error }) (Server, error) {
	var v Server
	err := row.Scan(&v.ID, &v.Kind, &v.Name, &v.URL, &v.APIKey, &v.RootFolder, &v.QualityProfileID, &v.QualityProfileName, &v.SeasonFolders, &v.SearchOnAdd, &v.Enabled, &v.CreatedAt, &v.UpdatedAt)
	return v, err
}

// Servers lists every connection, oldest first.
func (s *Store) Servers() ([]Server, error) {
	rows, err := s.db.Query(`SELECT ` + serverCols + ` FROM servers ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Server{}
	for rows.Next() {
		v, err := scanServer(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// FirstServer returns the oldest connection of a kind, or ok=false when there is none.
func (s *Store) FirstServer(kind string) (Server, bool, error) {
	v, err := scanServer(s.db.QueryRow(`SELECT `+serverCols+` FROM servers WHERE kind = ? ORDER BY id LIMIT 1`, kind))
	if errors.Is(err, sql.ErrNoRows) {
		return Server{}, false, nil
	}
	return v, err == nil, err
}

// SaveServer inserts the server (ID 0) or updates it, and returns the saved row.
func (s *Store) SaveServer(v Server) (Server, error) {
	t := now()
	if v.ID == 0 {
		res, err := s.db.Exec(`INSERT INTO servers (kind, name, url, api_key, root_folder, quality_profile_id, quality_profile_name, season_folders, search_on_add, enabled, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			v.Kind, v.Name, v.URL, v.APIKey, v.RootFolder, v.QualityProfileID, v.QualityProfileName, v.SeasonFolders, v.SearchOnAdd, v.Enabled, t, t)
		if err != nil {
			return Server{}, err
		}
		if v.ID, err = res.LastInsertId(); err != nil {
			return Server{}, err
		}
	} else if _, err := s.db.Exec(`UPDATE servers SET name = ?, url = ?, api_key = ?, root_folder = ?, quality_profile_id = ?, quality_profile_name = ?, season_folders = ?, search_on_add = ?, enabled = ?, updated_at = ? WHERE id = ?`,
		v.Name, v.URL, v.APIKey, v.RootFolder, v.QualityProfileID, v.QualityProfileName, v.SeasonFolders, v.SearchOnAdd, v.Enabled, t, v.ID); err != nil {
		return Server{}, err
	}
	return scanServer(s.db.QueryRow(`SELECT `+serverCols+` FROM servers WHERE id = ?`, v.ID))
}

// DeleteServer removes a connection.
func (s *Store) DeleteServer(id int64) error {
	_, err := s.db.Exec(`DELETE FROM servers WHERE id = ?`, id)
	return err
}
