package seerr

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Flixarr/flixarr/internal/secret"
	"github.com/Flixarr/flixarr/internal/store"
)

// seerrDB writes a small database laid out like Seerr's (only the tables and columns that matter).
func seerrDB(t *testing.T, newest int64, extra ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "db.sqlite3")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	stmts := []string{
		`CREATE TABLE migrations (id INTEGER PRIMARY KEY, timestamp bigint NOT NULL, name varchar NOT NULL)`,
		`CREATE TABLE user (id INTEGER PRIMARY KEY, email varchar, username varchar, plexUsername varchar, plexId integer, userType integer DEFAULT 1,
			permissions integer DEFAULT 0, avatar varchar, movieQuotaLimit integer, movieQuotaDays integer, tvQuotaLimit integer, tvQuotaDays integer,
			createdAt datetime DEFAULT CURRENT_TIMESTAMP, password varchar, plexToken varchar)`,
		`CREATE TABLE media (id INTEGER PRIMARY KEY, mediaType varchar, tmdbId integer, tvdbId integer, status integer, mediaAddedAt datetime)`,
		`CREATE TABLE media_request (id INTEGER PRIMARY KEY, status integer, mediaId integer, requestedById integer, modifiedById integer,
			createdAt datetime, updatedAt datetime, type varchar, is4k boolean DEFAULT 0, profileId integer)`,
		`CREATE TABLE season_request (id INTEGER PRIMARY KEY, seasonNumber integer, status integer, requestId integer)`,
		`CREATE TABLE issue (id INTEGER PRIMARY KEY, issueType integer, status integer, problemSeason integer, problemEpisode integer, mediaId integer,
			createdById integer, modifiedById integer, createdAt datetime, updatedAt datetime)`,
		`CREATE TABLE issue_comment (id INTEGER PRIMARY KEY, message text, userId integer, issueId integer, createdAt datetime)`,
		`INSERT INTO migrations (timestamp, name) VALUES (1603944374840, 'InitialMigration1603944374840'), (?, 'Newest')`,
		`INSERT INTO user VALUES (1, 'o@x.test', NULL, 'owner', 100, 1, 2, 'https://plex.tv/users/o/avatar', NULL, NULL, NULL, NULL, '2022-01-01 00:00:00', NULL, 'secret-token')`,
		`INSERT INTO user VALUES (2, 'e@x.test', NULL, 'eryn', 200, 1, 74211488, '', 10, 30, NULL, NULL, '2023-01-01 00:00:00', NULL, 'secret-token')`,
		`INSERT INTO user VALUES (3, 'g@x.test', 'grandma', NULL, NULL, 2, 32, '/avatarproxy/x', NULL, NULL, NULL, NULL, '2024-01-01 00:00:00', 'hash', NULL)`,
		`INSERT INTO media VALUES (1, 'movie', 603, NULL, 5, '2023-02-01 10:00:00'), (2, 'tv', 1396, 81189, 4, NULL), (3, 'movie', 605, NULL, 7, NULL)`,
		`INSERT INTO media_request VALUES (1, 2, 1, 2, 1, '2023-02-01 00:00:00', '2023-02-01 00:05:00', 'movie', 0, 1),
			(2, 2, 2, 3, NULL, '2023-03-01 00:00:00', '2023-03-01 00:00:00', 'tv', 0, 7),
			(3, 2, 3, 2, 1, '2023-04-01 00:00:00', '2023-04-01 00:00:00', 'movie', 0, 1),
			(4, 2, 1, 99, NULL, '2023-05-01 00:00:00', '2023-05-01 00:00:00', 'movie', 0, 1)`,
		`INSERT INTO season_request (seasonNumber, status, requestId) VALUES (2, 2, 2), (1, 2, 2)`,
		`INSERT INTO issue VALUES (1, 2, 1, 0, 0, 1, 2, NULL, '2023-06-01 00:00:00', '2023-06-01 00:00:00')`,
		`INSERT INTO issue_comment (message, userId, issueId, createdAt) VALUES ('Way too quiet', 2, 1, '2023-06-01 00:00:00'), ('On it', 1, 1, '2023-06-02 00:00:00')`,
	}
	stmts = append(stmts, extra...)
	for _, q := range stmts {
		var err error
		if strings.Contains(q, "(?, 'Newest')") {
			_, err = db.Exec(q, newest)
		} else {
			_, err = db.Exec(q)
		}
		if err != nil {
			t.Fatalf("%s: %v", q[:40], err)
		}
	}
	return path
}

func TestFileFetch(t *testing.T) {
	f := &File{DB: seerrDB(t, newestLayout), Settings: []byte(`{"radarr":[{"name":"Radarr","hostname":"radarr","port":7878,"apiKey":"rk","isDefault":true}],"sonarr":[]}`)}
	d, err := f.Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Users) != 3 || d.Users[1].Name() != "eryn" || d.Users[1].MovieQuotaLimit != 10 || d.Users[2].Name() != "grandma" || d.Users[2].UserType != 2 {
		t.Fatalf("users %+v", d.Users)
	}
	if d.Users[0].CreatedAt != "2022-01-01T00:00:00.000Z" {
		t.Errorf("date %q", d.Users[0].CreatedAt)
	}
	// Request 4's person is gone from Seerr: left out.
	if len(d.Requests) != 3 {
		t.Fatalf("%d requests", len(d.Requests))
	}
	tv := d.Requests[1]
	if tv.Type != "tv" || tv.Media.TvdbID != 81189 || len(seasonsOf(tv)) != 2 || seasonsOf(tv)[0] != 1 || tv.RequestedBy.ID != 3 || tv.ModifiedBy != nil {
		t.Errorf("tv request %+v", tv)
	}
	if d.Requests[0].Media.MediaAddedAt != "2023-02-01T10:00:00.000Z" || d.Requests[0].ModifiedBy == nil || d.Requests[0].ModifiedBy.ID != 1 {
		t.Errorf("movie request %+v", d.Requests[0])
	}
	if len(d.Issues) != 1 || len(d.Issues[0].Comments) != 2 || d.Issues[0].Comments[1].User.ID != 1 || d.Issues[0].IssueType != 2 {
		t.Errorf("issues %+v", d.Issues)
	}
	if len(d.Radarr) != 1 || d.Radarr[0].Address() != "http://radarr:7878" {
		t.Errorf("servers %+v", d.Radarr)
	}
	if !strings.HasPrefix(d.Version, "backup file") {
		t.Errorf("version %q", d.Version)
	}
	// The file is only read.
	if fi, _ := os.Stat(f.DB + "-wal"); fi != nil && fi.Size() > 0 {
		t.Error("the database was written to")
	}
}

func TestFileRefusals(t *testing.T) {
	ctx := context.Background()
	junk := filepath.Join(t.TempDir(), "db.sqlite3")
	os.WriteFile(junk, []byte("not a database at all"), 0o600)
	if _, err := (&File{DB: junk}).Fetch(ctx); err != ErrNotSeerr {
		t.Errorf("junk: %v", err)
	}
	if _, err := (&File{DB: seerrDB(t, newestLayout+1)}).Fetch(ctx); err == nil || !strings.Contains(err.Error(), "newer") {
		t.Errorf("newer layout: %v", err)
	}
	if _, err := (&File{DB: seerrDB(t, newestLayout, `ALTER TABLE issue DROP COLUMN problemSeason`)}).Fetch(ctx); err == nil || !strings.Contains(err.Error(), "issue.problemSeason") {
		t.Errorf("missing column: %v", err)
	}
	if _, err := (&File{DB: seerrDB(t, newestLayout), Settings: []byte("{nope")}).Fetch(ctx); err == nil {
		t.Error("bad settings.json accepted")
	}
}

// The file goes through the same preview and import as the API.
func TestFileImport(t *testing.T) {
	dir := t.TempDir()
	box, _ := secret.Load(dir)
	st, err := store.Open(dir, box)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	st.SavePlexItem(store.PlexItem{Type: "movie", TmdbID: 603, RatingKey: "1", SeenAt: "2026-01-01T00:00:00Z"})
	im := &Importer{Store: st}
	res, err := im.Run(context.Background(), &File{DB: seerrDB(t, newestLayout)})
	if err != nil {
		t.Fatal(err)
	}
	if res.Users != 3 || res.Requests != 3 || res.Issues != 1 || len(res.Servers) != 0 {
		t.Fatalf("result %+v", res)
	}
	if r, _ := st.RequestsForTitle("movie", 603); len(r) != 1 || r[0].Status != "available" {
		t.Errorf("603 %+v", r)
	}
	if r, _ := st.RequestsForTitle("series", 1396); len(r) != 1 || r[0].Status != "expired" || r[0].Mode != "seasons" {
		t.Errorf("1396 %+v", r)
	}
	gm, err := st.LocalUserByName("grandma")
	if err != nil || st.PasswordHash(gm.ID) != "" || gm.Avatar != "" {
		t.Fatalf("grandma %+v %v", gm, err)
	}
}
