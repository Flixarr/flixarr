package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/Flixarr/flixarr/internal/images"
	"github.com/Flixarr/flixarr/internal/jobs"
	"github.com/Flixarr/flixarr/internal/meta"
	"github.com/Flixarr/flixarr/internal/store"
)

// ---------- cache (Settings → Cache) ----------

type cacheState struct {
	Settings meta.Config       `json:"settings"`
	Defaults meta.Config       `json:"defaults"` // what "Default" resets to
	Kinds    []store.CacheKind `json:"kinds"`
	Counters meta.Counters     `json:"counters"`
	Images   imageState        `json:"images"`
	Avatars  rowState          `json:"avatars"`
}

// rowState is a Storage row that isn't a kind of saved result: what's saved and its lookups.
type rowState struct {
	images.Stats
	meta.HitMiss
}

type imageState struct {
	Enabled     bool `json:"enabled"`
	Days        int  `json:"days"`
	DefaultDays int  `json:"defaultDays"`
	images.Stats
	meta.HitMiss
}

func (s *Server) cacheState() (cacheState, error) {
	stats, err := s.Store.CacheStats()
	if err != nil {
		return cacheState{}, err
	}
	byKind := map[string]store.CacheKind{}
	for _, k := range stats {
		byKind[k.Kind] = k
	}
	kinds := make([]store.CacheKind, 0, len(meta.Kinds))
	for _, k := range meta.Kinds { // always every kind, in display order, even when empty
		kinds = append(kinds, store.CacheKind{Kind: k, Count: byKind[k].Count, Bytes: byKind[k].Bytes})
	}
	return cacheState{
		Settings: s.Meta.Config(),
		Defaults: meta.DefaultConfig,
		Kinds:    kinds,
		Counters: s.Meta.Counters(),
		Images:   imageState{Enabled: s.Images.Enabled(), Days: s.Images.Days(), DefaultDays: images.DefaultDays, Stats: s.Images.Stats(), HitMiss: s.Images.Images.Load()},
		Avatars:  rowState{Stats: s.Images.AvatarStats(), HitMiss: s.Images.Avatars.Load()},
	}, nil
}

func (s *Server) writeCache(w http.ResponseWriter) {
	st, err := s.cacheState()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Database error.")
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) cacheInfo(w http.ResponseWriter, r *http.Request, _ *store.User) { s.writeCache(w) }

func (s *Server) cacheSettings(w http.ResponseWriter, r *http.Request, _ *store.User) {
	var in struct {
		meta.Config
		Images struct {
			Enabled bool `json:"enabled"`
			Days    int  `json:"days"`
		} `json:"images"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "Bad request.")
		return
	}
	if err := s.Meta.SetConfig(in.Config); err != nil {
		writeErr(w, http.StatusBadRequest, capitalize(err.Error())+".")
		return
	}
	if err := s.Images.SetConfig(in.Images.Enabled, in.Images.Days); err != nil {
		writeErr(w, http.StatusBadRequest, capitalize(err.Error())+".")
		return
	}
	s.writeCache(w)
}

// cacheFlush clears one kind of saved results, "images", "avatars", or "all" (every saved result, image and avatar).
func (s *Server) cacheFlush(w http.ResponseWriter, r *http.Request, _ *store.User) {
	kind := r.PathValue("kind")
	var err error
	switch {
	case kind == "images":
		_, err = s.Images.Flush()
	case kind == "avatars":
		_, err = s.Images.FlushAvatars()
	case kind == "all":
		if _, err = s.Store.CacheFlush(""); err == nil {
			_, err = s.Images.Flush()
		}
		if err == nil {
			_, err = s.Images.FlushAvatars()
		}
	case contains(meta.Kinds, kind):
		_, err = s.Store.CacheFlush(kind)
	default:
		writeErr(w, http.StatusNotFound, "Unknown cache.")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Couldn't clear the cache.")
		return
	}
	s.writeCache(w)
}

// image serves a TMDB image from the image cache, downloading it the first time.
func (s *Server) image(w http.ResponseWriter, r *http.Request, _ *store.User) {
	size, file := r.PathValue("size"), r.PathValue("file")
	if !images.Valid(size, file) {
		writeErr(w, http.StatusNotFound, "Not found.")
		return
	}
	if !s.Images.Enabled() {
		http.Redirect(w, r, "https://image.tmdb.org/t/p/"+size+"/"+file, http.StatusFound)
		return
	}
	p, err := s.Images.Path(r.Context(), size, file)
	if err != nil {
		s.Log.Warn("image cache", "err", err)
		http.Redirect(w, r, "https://image.tmdb.org/t/p/"+size+"/"+file, http.StatusFound)
		return
	}
	w.Header().Set("Cache-Control", "private, max-age=2592000, immutable") // TMDB image paths never change content
	http.ServeFile(w, r, p)
}

// avatar serves someone's Plex avatar from the avatar cache (?u=<plex.tv address>), downloading it the
// first time. plex.tv answers avatars with an uncached redirect, so browsers would ask it on every view.
func (s *Server) avatar(w http.ResponseWriter, r *http.Request, _ *store.User) {
	u := r.URL.Query().Get("u")
	if !images.AvatarValid(u) {
		writeErr(w, http.StatusNotFound, "Not found.")
		return
	}
	if !s.Store.AvatarKnown(u) { // only people's own pictures are saved, so nobody can fill the disk with other addresses
		http.Redirect(w, r, u, http.StatusFound)
		return
	}
	p, err := s.Images.AvatarPath(r.Context(), u)
	if err != nil {
		s.Log.Warn("avatar cache", "err", err)
		http.Redirect(w, r, u, http.StatusFound)
		return
	}
	w.Header().Set("Cache-Control", "private, max-age=2592000, immutable") // a new picture has a new address
	http.ServeFile(w, r, p)
}

// ---------- jobs (Settings → Jobs) ----------

func (s *Server) jobList(w http.ResponseWriter, r *http.Request, _ *store.User) {
	writeJSON(w, http.StatusOK, s.Jobs.List())
}

func (s *Server) jobSet(w http.ResponseWriter, r *http.Request, _ *store.User) {
	var in struct {
		Schedule string `json:"schedule"`
		Enabled  bool   `json:"enabled"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "Bad request.")
		return
	}
	if err := s.Jobs.Set(r.PathValue("id"), strings.TrimSpace(in.Schedule), in.Enabled); err != nil {
		if errors.Is(err, jobs.ErrUnknown) {
			writeErr(w, http.StatusNotFound, "Unknown job.")
			return
		}
		writeErr(w, http.StatusBadRequest, capitalize(err.Error())+".")
		return
	}
	writeJSON(w, http.StatusOK, s.Jobs.List())
}

func (s *Server) jobRun(w http.ResponseWriter, r *http.Request, _ *store.User) {
	if err := s.Jobs.RunNow(s.BaseCtx, r.PathValue("id")); err != nil {
		writeErr(w, http.StatusNotFound, "Unknown job.")
		return
	}
	writeJSON(w, http.StatusOK, s.Jobs.List())
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
