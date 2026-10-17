package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/Flixarr/flixarr/internal/seerr"
	"github.com/Flixarr/flixarr/internal/store"
)

// Settings → Import (and the first-time setup step): bring users, requests, issues and Radarr/Sonarr
// over from Seerr, Overseerr or Jellyseerr. Preview reads only; Import runs in the background.

func (s *Server) seerrClient(w http.ResponseWriter, r *http.Request) (*seerr.Client, bool) {
	var in struct {
		URL string `json:"url"`
		Key string `json:"key"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "Bad request.")
		return nil, false
	}
	c, err := seerr.NewClient(in.URL, in.Key)
	if err != nil {
		writeErr(w, http.StatusBadRequest, capitalize(err.Error())+".")
		return nil, false
	}
	return c, true
}

func (s *Server) seerrState(w http.ResponseWriter, r *http.Request, _ *store.User) {
	writeJSON(w, http.StatusOK, s.Seerr.State())
}

func (s *Server) seerrPreview(w http.ResponseWriter, r *http.Request, _ *store.User) {
	c, ok := s.seerrClient(w, r)
	if !ok {
		return
	}
	p, _, _, err := s.Seerr.Plan(r.Context(), c)
	if err != nil {
		writeErr(w, http.StatusBadGateway, capitalize(err.Error())+".")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) seerrImport(w http.ResponseWriter, r *http.Request, _ *store.User) {
	c, ok := s.seerrClient(w, r)
	if !ok {
		return
	}
	if err := s.Seerr.Start(c, nil); err != nil {
		writeErr(w, http.StatusConflict, capitalize(err.Error())+".")
		return
	}
	writeJSON(w, http.StatusAccepted, s.Seerr.State())
}

// ---------- from a backup file (Seerr already gone) ----------

// seerrFiles is where an uploaded Seerr database (and settings.json) waits between the preview and the
// import. Importing removes them; a new upload replaces them.
func (s *Server) seerrFiles() (db, settings string) {
	dir := filepath.Join(s.Backups.DataDir, "import")
	return filepath.Join(dir, "seerr.sqlite3"), filepath.Join(dir, "seerr-settings.json")
}

func (s *Server) seerrFileSource() (*seerr.File, error) {
	db, settings := s.seerrFiles()
	if _, err := os.Stat(db); err != nil {
		return nil, errors.New("upload Seerr's database first")
	}
	f := &seerr.File{DB: db}
	f.Settings, _ = os.ReadFile(settings)
	return f, nil
}

// seerrFilePreview takes the uploaded db.sqlite3 (form field "db") and optional settings.json ("settings")
// and shows what importing them would do.
func (s *Server) seerrFilePreview(w http.ResponseWriter, r *http.Request, _ *store.User) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<30)
	mr, err := r.MultipartReader()
	if err != nil {
		writeErr(w, http.StatusBadRequest, "Upload Seerr's db.sqlite3.")
		return
	}
	db, settings := s.seerrFiles()
	if err := os.MkdirAll(filepath.Dir(db), 0o700); err != nil {
		writeErr(w, http.StatusInternalServerError, "Couldn't save the upload.")
		return
	}
	os.Remove(settings) // a settings.json from an earlier upload doesn't belong to this one
	gotDB := false
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			writeErr(w, http.StatusBadRequest, "The upload stopped part way. Try again.")
			return
		}
		var to string
		switch part.FormName() {
		case "db":
			to, gotDB = db, true
		case "settings":
			to = settings
		default:
			continue
		}
		if err := saveUpload(to, part); err != nil {
			writeErr(w, http.StatusBadRequest, "The upload stopped part way. Try again.")
			return
		}
	}
	if !gotDB {
		writeErr(w, http.StatusBadRequest, "Upload Seerr's db.sqlite3.")
		return
	}
	src, _ := s.seerrFileSource()
	p, _, _, err := s.Seerr.Plan(r.Context(), src)
	if err != nil {
		writeErr(w, http.StatusBadRequest, capitalize(err.Error())+".")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func saveUpload(to string, from io.Reader) error {
	tmp := to + ".part"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, from)
	f.Close()
	if err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, to)
}

// seerrFileImport imports the uploaded database in the background and removes the upload afterwards.
func (s *Server) seerrFileImport(w http.ResponseWriter, r *http.Request, _ *store.User) {
	src, err := s.seerrFileSource()
	if err != nil {
		writeErr(w, http.StatusBadRequest, capitalize(err.Error())+".")
		return
	}
	db, settings := s.seerrFiles()
	if err := s.Seerr.Start(src, func() { os.Remove(db); os.Remove(settings) }); err != nil {
		writeErr(w, http.StatusConflict, capitalize(err.Error())+".")
		return
	}
	writeJSON(w, http.StatusAccepted, s.Seerr.State())
}
