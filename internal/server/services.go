package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/Flixarr/flixarr/internal/arr"
	"github.com/Flixarr/flixarr/internal/store"
)

// ---------- Radarr / Sonarr connections (Settings → Services) ----------
// Servers are stored as a list; this screen manages the first one of each kind.

// serviceView is a saved connection as the browser sees it: the API key never leaves the server.
type serviceView struct {
	store.Server
	APIKeySet bool          `json:"apiKeySet"`
	APIKeyEnd string        `json:"apiKeyEnd"` // last 4 characters, so it can be recognized
	Status    *arr.Snapshot `json:"status"`    // latest card data from the refresh-status job
}

func (s *Server) viewOf(v store.Server) serviceView {
	end := v.APIKey
	if len(end) > 4 {
		end = end[len(end)-4:]
	}
	sv := serviceView{Server: v, APIKeySet: v.APIKey != "", APIKeyEnd: end}
	if snap, ok := s.Arr.Get(v.ID); ok {
		sv.Status = &snap
	}
	return sv
}

type serviceInput struct {
	Name             string `json:"name"`
	URL              string `json:"url"`
	APIKey           string `json:"apiKey"` // empty = keep the saved key
	RootFolder       string `json:"rootFolder"`
	QualityProfileID int    `json:"qualityProfileId"`
	SeasonFolders    bool   `json:"seasonFolders"`
	SearchOnAdd      bool   `json:"searchOnAdd"`
	Enabled          bool   `json:"enabled"`
}

func (s *Server) services(w http.ResponseWriter, r *http.Request, _ *store.User) {
	out := map[string]*serviceView{}
	for kind := range arr.Kinds {
		v, ok, err := s.Store.FirstServer(kind)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "Database error.")
			return
		}
		out[kind] = nil
		if ok {
			sv := s.viewOf(v)
			out[kind] = &sv
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// client builds a client from the submitted fields. The saved key is only reused for the saved
// address, so a key is never sent to a new address the admin hasn't typed it in for.
func (s *Server) serviceClient(kind string, in serviceInput) (*arr.Client, store.Server, bool, error) {
	if _, ok := arr.Kinds[kind]; !ok {
		return nil, store.Server{}, false, errors.New("unknown service")
	}
	u, err := arr.NormalizeURL(in.URL)
	if err != nil {
		return nil, store.Server{}, false, err
	}
	saved, ok, err := s.Store.FirstServer(kind)
	if err != nil {
		return nil, store.Server{}, false, errors.New("database error")
	}
	key := strings.TrimSpace(in.APIKey)
	if key == "" && ok && saved.URL == u {
		key = saved.APIKey
	}
	if key == "" {
		return nil, store.Server{}, false, errors.New("enter the API key (" + arr.Kinds[kind] + " → Settings → General)")
	}
	return &arr.Client{Kind: kind, URL: u, APIKey: key}, saved, ok, nil
}

func decodeService(w http.ResponseWriter, r *http.Request) (serviceInput, bool) {
	var in serviceInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "Bad request.")
		return in, false
	}
	return in, true
}

// serviceTest checks the submitted address and key and returns the version, profiles and folders.
func (s *Server) serviceTest(w http.ResponseWriter, r *http.Request, _ *store.User) {
	in, ok := decodeService(w, r)
	if !ok {
		return
	}
	c, _, _, err := s.serviceClient(r.PathValue("kind"), in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, capitalize(err.Error())+".")
		return
	}
	info, err := c.Test(r.Context())
	if err != nil {
		writeErr(w, http.StatusBadGateway, capitalize(err.Error())+".")
		return
	}
	writeJSON(w, http.StatusOK, info)
}

// serviceSave tests the connection, checks the picked folder and profile exist there, then saves.
func (s *Server) serviceSave(w http.ResponseWriter, r *http.Request, _ *store.User) {
	in, ok := decodeService(w, r)
	if !ok {
		return
	}
	kind := r.PathValue("kind")
	c, saved, exists, err := s.serviceClient(kind, in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, capitalize(err.Error())+".")
		return
	}
	info, err := c.Test(r.Context())
	if err != nil {
		writeErr(w, http.StatusBadGateway, "Not saved: "+err.Error()+".")
		return
	}
	v := saved
	if !exists {
		v = store.Server{Kind: kind}
	}
	v.Name, v.URL, v.APIKey = strings.TrimSpace(in.Name), c.URL, c.APIKey
	if v.Name == "" {
		v.Name = arr.Kinds[kind]
	}
	if len(v.Name) > 60 {
		v.Name = v.Name[:60]
	}
	v.SeasonFolders, v.SearchOnAdd, v.Enabled = in.SeasonFolders, in.SearchOnAdd, in.Enabled
	v.RootFolder, v.QualityProfileID, v.QualityProfileName = "", 0, ""
	for _, f := range info.Folders {
		if f.Path == in.RootFolder {
			v.RootFolder = f.Path
		}
	}
	for _, p := range info.Profiles {
		if p.ID == in.QualityProfileID {
			v.QualityProfileID, v.QualityProfileName = p.ID, p.Name
		}
	}
	if v.RootFolder == "" {
		writeErr(w, http.StatusBadRequest, "Pick a root folder.")
		return
	}
	if v.QualityProfileID == 0 {
		writeErr(w, http.StatusBadRequest, "Pick a default quality.")
		return
	}
	v, err = s.Store.SaveServer(v)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Database error.")
		return
	}
	s.Log.Info("service saved", "kind", kind, "url", v.URL)
	s.Arr.RefreshOne(r.Context(), v)
	writeJSON(w, http.StatusOK, s.viewOf(v))
}

// serviceDiscover looks for Radarr or Sonarr on the usual addresses and returns what answered.
func (s *Server) serviceDiscover(w http.ResponseWriter, r *http.Request, _ *store.User) {
	kind := r.PathValue("kind")
	if _, known := arr.Kinds[kind]; !known {
		writeErr(w, http.StatusNotFound, "Unknown service.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"found": nonNil(arr.Discover(r.Context(), kind))})
}

func nonNil(f []arr.Found) []arr.Found {
	if f == nil {
		return []arr.Found{}
	}
	return f
}

// serviceDelete removes the connection.
func (s *Server) serviceDelete(w http.ResponseWriter, r *http.Request, _ *store.User) {
	kind := r.PathValue("kind")
	if _, known := arr.Kinds[kind]; !known {
		writeErr(w, http.StatusNotFound, "Unknown service.")
		return
	}
	v, ok, err := s.Store.FirstServer(kind)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Database error.")
		return
	}
	if ok {
		if err := s.Store.DeleteServer(v.ID); err != nil {
			writeErr(w, http.StatusInternalServerError, "Database error.")
			return
		}
		s.Arr.Forget(v.ID)
		s.Log.Info("service removed", "kind", kind)
	}
	w.WriteHeader(http.StatusNoContent)
}
