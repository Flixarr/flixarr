// Package store is Flixarr's SQLite database: settings, users and sessions.
package store

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite driver: no cgo, so one static binary for amd64 and arm64

	"github.com/Flixarr/flixarr/internal/secret"
)

//go:embed migrations/*.sql
var migrations embed.FS

// ErrNotFound is returned when a row doesn't exist.
var ErrNotFound = errors.New("not found")

// Store wraps the database handle.
type Store struct {
	db  *sql.DB
	box *secret.Box // encrypts stored secrets (API keys)

	// OnAlert is told when the bell changes for an audience ("admins", "user:<id>", "everyone").
	OnAlert func(audience string)
}

// Open opens (or creates) flixarr.db inside dataDir and applies pending migrations.
func Open(dataDir string, box *secret.Box) (*Store, error) {
	dsn := "file:" + filepath.Join(dataDir, "flixarr.db") + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // SQLite has one writer; a single connection avoids "database is locked"
	s := &Store{db: db, box: box}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	if err := s.sealSecretSettings("plex_admin_token", "plex_server_token"); err != nil {
		db.Close()
		return nil, fmt.Errorf("encrypt settings: %w", err)
	}
	if err := s.sealServerKeys(); err != nil {
		db.Close()
		return nil, fmt.Errorf("encrypt keys: %w", err)
	}
	return s, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (name TEXT PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		return err
	}
	names, err := fs.Glob(migrations, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(names)
	for _, name := range names {
		var done int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE name = ?`, name).Scan(&done); err != nil {
			return err
		}
		if done > 0 {
			continue
		}
		body, err := migrations.ReadFile(name)
		if err != nil {
			return err
		}
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		for _, stmt := range strings.Split(string(body), ";\n") {
			if strings.TrimSpace(stmt) == "" {
				continue
			}
			if _, err := tx.Exec(stmt); err != nil {
				tx.Rollback()
				return fmt.Errorf("%s: %w", name, err)
			}
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations (name, applied_at) VALUES (?, ?)`, name, now()); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func now() string { return time.Now().UTC().Format(time.RFC3339) }

// ---------- settings ----------

// Setting returns a setting value, or def when it isn't set.
func (s *Store) Setting(key, def string) string {
	var v string
	if err := s.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&v); err != nil {
		return def
	}
	return v
}

// SetSetting stores a setting value.
func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

// ---------- users ----------

// User is a person who can sign in.
type User struct {
	ID       int64    `json:"id"`
	PlexID   int64    `json:"plexId"`
	Username string   `json:"username"`
	Email    string   `json:"email"`
	Avatar   string   `json:"avatar"`
	Role     string   `json:"role"`  // User, Trusted, Manager or Custom: worked out from Perms (admin aside)
	Perms    []string `json:"perms"` // see perms.go
	Owner    bool     `json:"owner"` // the first admin; can't be removed or lose admin
	Local    bool     `json:"local"` // a username-and-password account (no Plex)
	// ViewAs is the role a test install's admin is viewing the app as ("" = as themselves); RealAdmin says
	// the person behind it is an admin, whatever they're viewing as.
	ViewAs    string `json:"viewAs,omitempty"`
	RealAdmin bool   `json:"-"`
	// ManagesUsers is CanManageUsers, filled in for the signed-in person (/api/me).
	ManagesUsers bool `json:"managesUsers,omitempty"`
	// Their own region and language for titles, dates and "upcoming" ("" = the server's).
	Region   string `json:"region"`
	Language string `json:"language"`
	// Request limits per 7 days (0 = unlimited; admins have none).
	MovieLimit  int    `json:"movieLimit"`
	SeasonLimit int    `json:"seasonLimit"`
	CreatedAt   string `json:"createdAt"`
	LastSeen    string `json:"lastSeen,omitempty"` // Users list and profiles only
}

// UserCount returns how many users exist (0 means first-run setup is needed).
func (s *Store) UserCount() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

const userCols = `id, plex_id, username, email, avatar, role, perms, created_at, id = (SELECT MIN(id) FROM users), movie_limit, season_limit, region, language`

func scanUser(row interface{ Scan(...any) error }) (*User, error) {
	u := &User{}
	var perms string
	if err := row.Scan(&u.ID, &u.PlexID, &u.Username, &u.Email, &u.Avatar, &u.Role, &perms, &u.CreatedAt, &u.Owner, &u.MovieLimit, &u.SeasonLimit, &u.Region, &u.Language); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	u.Perms = splitPerms(perms)
	u.Role = RoleOf(u.Perms) // worked out from the switches: the stored label can be from older presets
	u.Local = u.PlexID == 0
	return u, nil
}

// UserByPlexID finds a user by their plex.tv account id.
func (s *Store) UserByPlexID(plexID int64) (*User, error) {
	return scanUser(s.db.QueryRow(`SELECT `+userCols+` FROM users WHERE plex_id = ?`, plexID))
}

// AvatarKnown reports whether u is someone's saved avatar address.
func (s *Store) AvatarKnown(u string) bool {
	var n int
	s.db.QueryRow(`SELECT COUNT(*) FROM users WHERE avatar = ?`, u).Scan(&n)
	return n > 0
}

// CreateUser inserts a user and returns it.
func (s *Store) CreateUser(u User) (*User, error) {
	if u.Perms == nil {
		u.Perms = RolePerms[u.Role]
		if u.Role == "Admin" {
			u.Perms = AllPerms // shorthand: an admin can do everything
		}
	}
	u.Perms = cleanPerms(u.Perms)
	res, err := s.db.Exec(`INSERT INTO users (plex_id, username, email, avatar, role, perms, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		u.PlexID, u.Username, u.Email, u.Avatar, RoleOf(u.Perms), strings.Join(u.Perms, ","), now())
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return scanUser(s.db.QueryRow(`SELECT `+userCols+` FROM users WHERE id = ?`, id))
}

// ErrNameTaken: a local account already has that username.
var ErrNameTaken = errors.New("that username is taken")

// CreateLocalUser adds a username-and-password account; hash comes from the password package.
func (s *Store) CreateLocalUser(username, email, role, hash string) (*User, error) {
	var n int
	s.db.QueryRow(`SELECT COUNT(*) FROM users WHERE plex_id = 0 AND lower(username) = lower(?)`, username).Scan(&n)
	if n > 0 {
		return nil, ErrNameTaken
	}
	u, err := s.CreateUser(User{Username: username, Email: email, Role: role})
	if err != nil {
		return nil, err
	}
	return u, s.SetPassword(u.ID, hash)
}

// LocalLogin finds a local account by username (any case) and returns it with its password hash.
func (s *Store) LocalLogin(username string) (*User, string, error) {
	var hash string
	var id int64
	err := s.db.QueryRow(`SELECT id, password_hash FROM users WHERE plex_id = 0 AND lower(username) = lower(?)`, username).Scan(&id, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", ErrNotFound
	}
	if err != nil {
		return nil, "", err
	}
	u, err := scanUser(s.db.QueryRow(`SELECT `+userCols+` FROM users WHERE id = ?`, id))
	return u, hash, err
}

// PasswordHash returns a user's stored password hash ("" for Plex accounts).
func (s *Store) PasswordHash(id int64) string {
	var h string
	s.db.QueryRow(`SELECT password_hash FROM users WHERE id = ?`, id).Scan(&h)
	return h
}

// SetPassword stores a new password hash and signs the account out everywhere.
func (s *Store) SetPassword(id int64, hash string) error {
	if _, err := s.db.Exec(`UPDATE users SET password_hash = ? WHERE id = ? AND plex_id = 0`, hash, id); err != nil {
		return err
	}
	_, err := s.db.Exec(`DELETE FROM sessions WHERE user_id = ?`, id)
	return err
}

// SetUserLocale saves a person's own region and language ("" = the server's).
func (s *Store) SetUserLocale(id int64, region, language string) error {
	_, err := s.db.Exec(`UPDATE users SET region = ?, language = ? WHERE id = ?`, region, language, id)
	return err
}

// UpdatePlexProfile refreshes the name, email and avatar from plex.tv on each sign-in.
func (s *Store) UpdatePlexProfile(id int64, username, email, avatar string) error {
	_, err := s.db.Exec(`UPDATE users SET username = ?, email = ?, avatar = ? WHERE id = ?`, username, email, avatar, id)
	return err
}

// ---------- sessions ----------

// SessionTTL is how long a sign-in lasts.
const SessionTTL = 30 * 24 * time.Hour

func hashToken(t string) string { h := sha256.Sum256([]byte(t)); return hex.EncodeToString(h[:]) }

// CreateSession returns a new random session token for the user. Only its hash is stored.
func (s *Store) CreateSession(userID int64) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := hex.EncodeToString(b)
	_, err := s.db.Exec(`INSERT INTO sessions (token_hash, user_id, expires_at, created_at) VALUES (?, ?, ?, ?)`,
		hashToken(token), userID, time.Now().Add(SessionTTL).UTC().Format(time.RFC3339), now())
	return token, err
}

// SessionUser returns the user for a session token, if the session is still valid.
func (s *Store) SessionUser(token string) (*User, error) {
	row := s.db.QueryRow(`SELECT u.id, u.plex_id, u.username, u.email, u.avatar, u.role, u.perms, u.created_at, u.id = (SELECT MIN(id) FROM users), u.movie_limit, u.season_limit, u.region, u.language
		FROM sessions s JOIN users u ON u.id = s.user_id WHERE s.token_hash = ? AND s.expires_at > ?`, hashToken(token), now())
	u, err := scanUser(row)
	if err == nil {
		t := time.Now().UTC()
		s.db.Exec(`UPDATE users SET last_seen = ? WHERE id = ? AND last_seen < ?`, t.Format(time.RFC3339), u.ID, t.Add(-5*time.Minute).Format(time.RFC3339))
	}
	return u, err
}

// LastSeen is when someone last used Flixarr ("" = never since it was tracked).
func (s *Store) LastSeen(id int64) string {
	var t string
	s.db.QueryRow(`SELECT last_seen FROM users WHERE id = ?`, id).Scan(&t)
	return t
}

// DeleteSession signs a session out.
func (s *Store) DeleteSession(token string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE token_hash = ?`, hashToken(token))
	return err
}

// ---------- secret settings ----------

// SecretSetting returns a setting stored encrypted (Plex tokens), decrypted; def when it isn't set
// or can't be decrypted.
func (s *Store) SecretSetting(key, def string) string {
	v, err := s.box.Open(s.Setting(key, ""))
	if err != nil || v == "" {
		return def
	}
	return v
}

// SetSecretSetting stores a setting encrypted.
func (s *Store) SetSecretSetting(key, value string) error {
	return s.SetSetting(key, s.box.Seal(value))
}

// sealSecretSettings encrypts secret settings saved in plain text before encryption existed.
func (s *Store) sealSecretSettings(keys ...string) error {
	for _, k := range keys {
		v := s.Setting(k, "")
		if v != "" && !secret.Sealed(v) {
			if err := s.SetSecretSetting(k, v); err != nil {
				return err
			}
		}
	}
	return nil
}

// ---------- backups ----------

// BackupTo writes a consistent copy of the database to path (which must not exist), without the
// Flixarr API cache: that's rebuilt on its own and would only make backups big.
func (s *Store) BackupTo(path string) error {
	if _, err := s.db.Exec(`VACUUM INTO ?`, path); err != nil {
		return err
	}
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		return err
	}
	defer db.Close()
	if _, err := db.Exec(`DELETE FROM cache`); err != nil {
		return err
	}
	_, err = db.Exec(`VACUUM`)
	return err
}
