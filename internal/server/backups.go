package server

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"syscall"
	"time"

	"github.com/Flixarr/flixarr/internal/backup"
	"github.com/Flixarr/flixarr/internal/store"
)

func (s *Server) backupList(w http.ResponseWriter, r *http.Request, _ *store.User) {
	writeJSON(w, http.StatusOK, map[string]any{"backups": s.Backups.List(), "keep": s.Backups.Keep()})
}

func (s *Server) backupCreate(w http.ResponseWriter, r *http.Request, _ *store.User) {
	b, err := s.Backups.Create("manual")
	if err != nil {
		s.Log.Error("backup", "err", err)
		writeErr(w, http.StatusInternalServerError, "Couldn't save a backup: "+err.Error()+".")
		return
	}
	writeJSON(w, http.StatusCreated, b)
}

func (s *Server) backupSettings(w http.ResponseWriter, r *http.Request, u *store.User) {
	var in struct {
		Keep int `json:"keep"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&in); err != nil || in.Keep < 1 || in.Keep > 60 {
		writeErr(w, http.StatusBadRequest, "Keep between 1 and 60 backups.")
		return
	}
	s.Store.SetSetting(backup.KeyKeep, strconv.Itoa(in.Keep))
	s.backupList(w, r, u)
}

func (s *Server) backupDownload(w http.ResponseWriter, r *http.Request, _ *store.User) {
	p, err := s.Backups.Path(r.PathValue("name"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "No such backup.")
		return
	}
	w.Header().Set("Content-Type", "application/vnd.sqlite3")
	w.Header().Set("Content-Disposition", `attachment; filename="`+r.PathValue("name")+`"`)
	http.ServeFile(w, r, p)
}

func (s *Server) backupDelete(w http.ResponseWriter, r *http.Request, _ *store.User) {
	if err := s.Backups.Delete(r.PathValue("name")); err != nil {
		writeErr(w, http.StatusNotFound, "No such backup.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// backupRestore stages the backup and restarts Flixarr in place (the same process image, so it works
// whatever the container's restart policy); the database is swapped before it opens.
func (s *Server) backupRestore(w http.ResponseWriter, r *http.Request, u *store.User) {
	name := r.PathValue("name")
	if err := s.Backups.PrepareRestore(name); err != nil {
		writeErr(w, http.StatusBadRequest, capitalize(err.Error())+".")
		return
	}
	s.Log.Warn("restoring a backup and restarting", "backup", name, "by", u.Username)
	w.WriteHeader(http.StatusAccepted)
	go func() {
		time.Sleep(700 * time.Millisecond) // let the answer reach the browser
		exe, err := os.Executable()
		if err == nil {
			err = syscall.Exec(exe, os.Args, os.Environ())
		}
		s.Log.Error("restart after restore failed; restart Flixarr to finish", "err", err)
		os.Exit(1)
	}()
}
