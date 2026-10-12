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
	ID        int64    `json:"id"`
	PlexID    int64    `json:"plexId"`
	Username  string   `json:"username"`
	Email     string   `json:"email"`
	Avatar    string   `json:"avatar"`
	Role      string   `json:"role"`  // Admin, Trusted, User or Custom: a label worked out from Perms
	Perms     []string `json:"perms"` // see perms.go
	Owner     bool     `json:"owner"` // the first admin; can't be removed or lose admin
	CreatedAt string   `json:"createdAt"`
}

// UserCount returns how many users exist (0 means first-run setup is needed).
func (s *Store) UserCount() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

const userCols = `id, plex_id, username, email, avatar, role, perms, created_at, id = (SELECT MIN(id) FROM users)`

func scanUser(row interface{ Scan(...any) error }) (*User, error) {
	u := &User{}
	var perms string
	if err := row.Scan(&u.ID, &u.PlexID, &u.Username, &u.Email, &u.Avatar, &u.Role, &perms, &u.CreatedAt, &u.Owner); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	u.Perms = splitPerms(perms)
	return u, nil
}

// UserByPlexID finds a user by their plex.tv account id.
func (s *Store) UserByPlexID(plexID int64) (*User, error) {
	return scanUser(s.db.QueryRow(`SELECT `+userCols+` FROM users WHERE plex_id = ?`, plexID))
}

// CreateUser inserts a user and returns it.
func (s *Store) CreateUser(u User) (*User, error) {
	if u.Perms == nil {
		u.Perms = RolePerms[u.Role]
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
	row := s.db.QueryRow(`SELECT u.id, u.plex_id, u.username, u.email, u.avatar, u.role, u.perms, u.created_at, u.id = (SELECT MIN(id) FROM users)
		FROM sessions s JOIN users u ON u.id = s.user_id WHERE s.token_hash = ? AND s.expires_at > ?`, hashToken(token), now())
	return scanUser(row)
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
