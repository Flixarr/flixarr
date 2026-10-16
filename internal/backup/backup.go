// Package backup makes daily copies of Flixarr's database, keeps the newest few, and restores one.
package backup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Flixarr/flixarr/internal/store"
)

// KeyKeep is how many backups to keep (Settings → Backups).
const KeyKeep = "backup_keep"

// Manager owns the backups folder inside the data folder.
type Manager struct {
	Store   *store.Store
	DataDir string
}

// Backup is one file in the backups folder.
type Backup struct {
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	Created string `json:"created"`
	Kind    string `json:"kind"` // scheduled | manual | before-restore
}

var nameRe = regexp.MustCompile(`^flixarr-(\d{8}-\d{6})(?:-(manual|before-restore))?\.db$`)

func (m *Manager) dir() string { return filepath.Join(m.DataDir, "backups") }

// Keep is how many backups are kept.
func (m *Manager) Keep() int {
	n, err := strconv.Atoi(m.Store.Setting(KeyKeep, "7"))
	if err != nil || n < 1 {
		return 7
	}
	return n
}

// Create makes a backup now. kind is "" for the scheduled one or "manual".
func (m *Manager) Create(kind string) (Backup, error) {
	if err := os.MkdirAll(m.dir(), 0o700); err != nil {
		return Backup{}, err
	}
	name := "flixarr-" + time.Now().UTC().Format("20060102-150405")
	if kind != "" {
		name += "-" + kind
	}
	name += ".db"
	path := filepath.Join(m.dir(), name)
	if err := m.Store.BackupTo(path); err != nil {
		os.Remove(path)
		return Backup{}, err
	}
	os.Chmod(path, 0o600)
	m.prune()
	for _, b := range m.List() {
		if b.Name == name {
			return b, nil
		}
	}
	return Backup{}, errors.New("backup disappeared")
}

// Run is the daily "Backup" job.
func (m *Manager) Run() (string, error) {
	b, err := m.Create("")
	if err != nil {
		return "", err
	}
	msg := fmt.Sprintf("Saved %s (%d KB)", b.Name, b.Size/1024)
	if o := m.LoadOffsite(); o.Folder != "" || o.S3.On() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		if err := m.CopyOffsite(ctx, b.Name); err != nil {
			return msg, fmt.Errorf("saved, but the offsite copy failed: %w", err)
		}
		msg += ", copied offsite"
	}
	return msg, nil
}

// List returns the backups, newest first.
func (m *Manager) List() []Backup {
	entries, _ := os.ReadDir(m.dir())
	out := []Backup{}
	for _, e := range entries {
		g := nameRe.FindStringSubmatch(e.Name())
		if g == nil {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		t, _ := time.Parse("20060102-150405", g[1])
		kind := g[2]
		if kind == "" {
			kind = "scheduled"
		}
		out = append(out, Backup{Name: e.Name(), Size: info.Size(), Created: t.Format(time.RFC3339), Kind: kind})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name > out[j].Name })
	return out
}

// prune keeps the newest Keep() backups (copies made before a restore are kept too, but count).
func (m *Manager) prune() {
	list := m.List()
	sort.Slice(list, func(i, j int) bool { return list[i].Created > list[j].Created })
	for i, b := range list {
		if i >= m.Keep() {
			os.Remove(filepath.Join(m.dir(), b.Name))
		}
	}
}

// Path returns a backup's file, refusing anything that isn't one.
func (m *Manager) Path(name string) (string, error) {
	if !nameRe.MatchString(name) || strings.ContainsAny(name, `/\`) {
		return "", errors.New("no such backup")
	}
	p := filepath.Join(m.dir(), name)
	if _, err := os.Stat(p); err != nil {
		return "", errors.New("no such backup")
	}
	return p, nil
}

// Delete removes a backup.
func (m *Manager) Delete(name string) error {
	p, err := m.Path(name)
	if err != nil {
		return err
	}
	return os.Remove(p)
}

// PrepareRestore stages a backup to replace the database on the next start (see ApplyRestore), after
// saving the current database as a before-restore backup so the restore can be undone.
func (m *Manager) PrepareRestore(name string) error {
	p, err := m.Path(name)
	if err != nil {
		return err
	}
	if _, err := m.Create("before-restore"); err != nil {
		return fmt.Errorf("couldn't save the current database first: %w", err)
	}
	return copyFile(p, filepath.Join(m.DataDir, "restore.db"))
}

// ApplyRestore runs before the database opens: if a restore is staged, it takes the database's place.
func ApplyRestore(dataDir string) (bool, error) {
	staged := filepath.Join(dataDir, "restore.db")
	if _, err := os.Stat(staged); err != nil {
		return false, nil
	}
	db := filepath.Join(dataDir, "flixarr.db")
	for _, ext := range []string{"-wal", "-shm"} {
		os.Remove(db + ext) // belongs to the database being replaced (already saved as a before-restore backup)
	}
	return true, os.Rename(staged, db)
}

func copyFile(from, to string) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := to + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, to)
}
