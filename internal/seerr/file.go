package seerr

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Source is where an import reads Seerr from: its API (Client) or a backup of its database (File).
type Source interface {
	Fetch(ctx context.Context) (*Data, error)
}

// File reads a Seerr, Overseerr or Jellyseerr database file (db/db.sqlite3 in its config folder), for
// people whose Seerr is already gone. Radarr and Sonarr aren't in the database; they come from the
// settings.json next to it, when given. The file is only read.
type File struct {
	DB       string // path to the uploaded db.sqlite3
	Settings []byte // settings.json, optional
}

// newestLayout is the newest Seerr database change this reader was checked against (the timestamp of
// Seerr's latest migration). A newer database may be laid out differently, so it's refused.
const newestLayout = 1789254652493

// The columns read from each table. Overseerr, Jellyseerr and every Seerr since have them.
var needed = map[string][]string{
	"user":           {"id", "email", "username", "plexUsername", "plexId", "userType", "permissions", "avatar", "movieQuotaLimit", "movieQuotaDays", "tvQuotaLimit", "tvQuotaDays", "createdAt"},
	"media":          {"id", "mediaType", "tmdbId", "tvdbId", "status"},
	"media_request":  {"id", "status", "mediaId", "requestedById", "modifiedById", "createdAt", "updatedAt", "type", "is4k", "profileId"},
	"season_request": {"seasonNumber", "requestId"},
	"issue":          {"id", "issueType", "status", "problemSeason", "problemEpisode", "mediaId", "createdById", "modifiedById", "createdAt", "updatedAt"},
	"issue_comment":  {"message", "userId", "issueId", "createdAt"},
	"migrations":     {"timestamp", "name"},
}

// ErrNotSeerr: the file isn't a Seerr database (or isn't a database at all).
var ErrNotSeerr = errors.New("that file isn't a Seerr database (it's db.sqlite3 in Seerr's config folder, under db)")

// Fetch reads the database.
func (f *File) Fetch(ctx context.Context) (*Data, error) {
	db, err := sql.Open("sqlite", "file:"+url.PathEscape(f.DB)+"?mode=ro&_pragma=query_only(1)")
	if err != nil {
		return nil, err
	}
	defer db.Close()
	layout, err := checkLayout(ctx, db)
	if err != nil {
		return nil, err
	}
	d := &Data{Version: "backup file"}
	if layout > 0 {
		d.Version = "backup file (" + time.UnixMilli(layout).UTC().Format("Jan 2006") + " layout)"
	}
	hasAdded := hasColumn(ctx, db, "media", "mediaAddedAt")

	users := map[int64]User{}
	rows, err := db.QueryContext(ctx, `SELECT id, COALESCE(email, ''), COALESCE(username, ''), COALESCE(plexUsername, ''), COALESCE(plexId, 0), userType,
		permissions, COALESCE(avatar, ''), COALESCE(movieQuotaLimit, 0), COALESCE(movieQuotaDays, 0), COALESCE(tvQuotaLimit, 0), COALESCE(tvQuotaDays, 0), createdAt FROM user ORDER BY id`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var u User
		var created string
		if err := rows.Scan(&u.ID, &u.Email, &u.Username, &u.PlexUsername, &u.PlexID, &u.UserType, &u.Permissions, &u.Avatar,
			&u.MovieQuotaLimit, &u.MovieQuotaDays, &u.TvQuotaLimit, &u.TvQuotaDays, &created); err != nil {
			rows.Close()
			return nil, err
		}
		u.CreatedAt = stamp(created)
		users[u.ID] = u
		d.Users = append(d.Users, u)
	}
	rows.Close()
	who := func(id sql.NullInt64) *User {
		if u, ok := users[id.Int64]; ok && id.Valid {
			return &u
		}
		return nil
	}

	media := map[int64]Media{}
	added := `NULL`
	if hasAdded {
		added = `mediaAddedAt`
	}
	rows, err = db.QueryContext(ctx, `SELECT id, mediaType, tmdbId, COALESCE(tvdbId, 0), status, COALESCE(`+added+`, '') FROM media`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id int64
		var m Media
		var at string
		if err := rows.Scan(&id, &m.MediaType, &m.TmdbID, &m.TvdbID, &m.Status, &at); err != nil {
			rows.Close()
			return nil, err
		}
		m.MediaAddedAt = stamp(at)
		media[id] = m
	}
	rows.Close()

	seasons := map[int64][]Season{}
	rows, err = db.QueryContext(ctx, `SELECT requestId, seasonNumber FROM season_request ORDER BY seasonNumber`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id int64
		var s Season
		if rows.Scan(&id, &s.SeasonNumber) == nil {
			seasons[id] = append(seasons[id], s)
		}
	}
	rows.Close()

	rows, err = db.QueryContext(ctx, `SELECT id, status, mediaId, requestedById, modifiedById, createdAt, updatedAt, type, is4k, COALESCE(profileId, 0) FROM media_request ORDER BY id`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var r Request
		var mediaID int64
		var by, mod sql.NullInt64
		var created, updated string
		if err := rows.Scan(&r.ID, &r.Status, &mediaID, &by, &mod, &created, &updated, &r.Type, &r.Is4k, &r.ProfileID); err != nil {
			rows.Close()
			return nil, err
		}
		u := who(by)
		m, ok := media[mediaID]
		if u == nil || !ok {
			continue // its person or title is gone from Seerr
		}
		r.RequestedBy, r.ModifiedBy, r.Media, r.Seasons = *u, who(mod), m, seasons[r.ID]
		r.CreatedAt, r.UpdatedAt = stamp(created), stamp(updated)
		d.Requests = append(d.Requests, r)
	}
	rows.Close()

	comments := map[int64][]Comment{}
	rows, err = db.QueryContext(ctx, `SELECT issueId, userId, message, createdAt FROM issue_comment ORDER BY id`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id int64
		var uid sql.NullInt64
		var c Comment
		var created string
		if rows.Scan(&id, &uid, &c.Message, &created) == nil {
			c.User, c.CreatedAt = who(uid), stamp(created)
			comments[id] = append(comments[id], c)
		}
	}
	rows.Close()

	rows, err = db.QueryContext(ctx, `SELECT id, issueType, status, problemSeason, problemEpisode, mediaId, createdById, modifiedById, createdAt, updatedAt FROM issue ORDER BY id`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var i Issue
		var mediaID int64
		var by, mod sql.NullInt64
		var created, updated string
		if err := rows.Scan(&i.ID, &i.IssueType, &i.Status, &i.ProblemSeason, &i.ProblemEpisode, &mediaID, &by, &mod, &created, &updated); err != nil {
			rows.Close()
			return nil, err
		}
		u := who(by)
		m, ok := media[mediaID]
		if u == nil || !ok {
			continue
		}
		i.CreatedBy, i.ModifiedBy, i.Media, i.Comments = *u, who(mod), m, comments[i.ID]
		i.CreatedAt, i.UpdatedAt = stamp(created), stamp(updated)
		d.Issues = append(d.Issues, i)
	}
	rows.Close()

	if len(f.Settings) > 0 {
		var s struct {
			Radarr []Server `json:"radarr"`
			Sonarr []Server `json:"sonarr"`
		}
		if err := json.Unmarshal(f.Settings, &s); err != nil {
			return nil, errors.New("that settings.json can't be read; leave it out and add Radarr and Sonarr by hand")
		}
		d.Radarr, d.Sonarr = s.Radarr, s.Sonarr
	}
	return d, nil
}

// checkLayout makes sure the file is a Seerr database this reader understands and returns its newest
// migration's timestamp.
func checkLayout(ctx context.Context, db *sql.DB) (int64, error) {
	if !hasColumn(ctx, db, "user", "permissions") || !hasColumn(ctx, db, "migrations", "timestamp") {
		return 0, ErrNotSeerr
	}
	for table, cols := range needed {
		for _, c := range cols {
			if !hasColumn(ctx, db, table, c) {
				return 0, fmt.Errorf("this Seerr database is laid out in a way Flixarr doesn't know (%s.%s is missing). Start Seerr once and import through its API instead", table, c)
			}
		}
	}
	var newest sql.NullInt64
	db.QueryRowContext(ctx, `SELECT MAX(timestamp) FROM migrations`).Scan(&newest)
	if newest.Int64 > newestLayout {
		return 0, errors.New("this Seerr database is newer than this version of Flixarr knows. Update Flixarr, or start Seerr once and import through its API")
	}
	return newest.Int64, nil
}

func hasColumn(ctx context.Context, db *sql.DB, table, col string) bool {
	rows, err := db.QueryContext(ctx, `SELECT name FROM pragma_table_info(?)`, table)
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var n string
		if rows.Scan(&n) == nil && n == col {
			return true
		}
	}
	return false
}

// stamp turns Seerr's stored times ("2023-02-01 00:00:00", UTC) into the API's form.
func stamp(s string) string {
	s = strings.TrimSpace(s)
	for _, layout := range []string{"2006-01-02 15:04:05.000", "2006-01-02 15:04:05", time.RFC3339Nano} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC().Format("2006-01-02T15:04:05.000Z")
		}
	}
	return s
}
