package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/Flixarr/flixarr/internal/arr"
	"github.com/Flixarr/flixarr/internal/profiles"
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
	// Flixarr's quality profiles on this server (added or not).
	Profiles []profiles.ProfileStatus `json:"profiles"`
	Warning  string                   `json:"warning,omitempty"` // e.g. saved, but adding profiles failed
	// When the refresh-status job runs next (nil when it's paused), for the card's countdown.
	NextUpdate    *time.Time `json:"nextUpdate"`
	UpdatesPaused bool       `json:"updatesPaused"`
}

func (s *Server) viewOf(v store.Server) serviceView {
	end := v.APIKey
	if len(end) > 4 {
		end = end[len(end)-4:]
	}
	sv := serviceView{Server: v, APIKeySet: v.APIKey != "", APIKeyEnd: end}
	if snap, ok := s.Arr.Get(v.ID); ok {
		// Stuck downloads link to their request (or to the title when nobody requested it in Flixarr).
		snap.Queue = append([]arr.QueueRow(nil), snap.Queue...)
		for i, q := range snap.Queue {
			if (q.State == "warning" || q.State == "error") && q.ArrID > 0 {
				if list, _ := s.Store.ActiveRequestsFor(q.Type, q.ArrID); len(list) > 0 {
					snap.Queue[i].RequestID = list[0].ID
				}
			}
		}
		sv.Status = &snap
	}
	sv.Profiles, _ = s.Profiles.Status(v)
	if sv.Profiles == nil {
		sv.Profiles = []profiles.ProfileStatus{}
	}
	for _, j := range s.Jobs.List() {
		if j.ID == "arr-status" {
			sv.UpdatesPaused = !j.Enabled
			if j.Enabled && !j.Next.IsZero() {
				n := j.Next
				// Idle last time (empty queue, healthy): the job skips it until 5 minutes have passed.
				if st := sv.Status; st != nil && st.Connected && len(st.Queue) == 0 && len(st.Health) == 0 {
					for n.Before(st.UpdatedAt.Add(5 * time.Minute)) {
						n = n.Add(time.Minute)
					}
				}
				sv.NextUpdate = &n
			}
		}
	}
	return sv
}

type serviceInput struct {
	Name             string `json:"name"`
	URL              string `json:"url"`
	WebURL           string `json:"webUrl"` // optional
	APIKey           string `json:"apiKey"` // empty = keep the saved key
	RootFolder       string `json:"rootFolder"`
	QualityProfileID int    `json:"qualityProfileId"`
	SeasonFolders    bool   `json:"seasonFolders"`
	SearchOnAdd      bool   `json:"searchOnAdd"`
	// First setup: Flixarr profiles to add right after saving, and one of them to use as the default quality.
	AddProfiles    []string `json:"addProfiles"`
	DefaultProfile string   `json:"defaultProfile"`
	Enabled        bool     `json:"enabled"`
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
	v.WebURL = ""
	if strings.TrimSpace(in.WebURL) != "" {
		if v.WebURL, err = arr.NormalizeURL(in.WebURL); err != nil {
			writeErr(w, http.StatusBadRequest, "Web address: "+err.Error()+".")
			return
		}
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
	if v.QualityProfileID == 0 && in.DefaultProfile == "" {
		writeErr(w, http.StatusBadRequest, "Pick a default quality.")
		return
	}
	if v.QualityProfileID == 0 && len(info.Profiles) > 0 { // replaced by the Flixarr profile below
		v.QualityProfileID, v.QualityProfileName = info.Profiles[0].ID, info.Profiles[0].Name
	}
	v, err = s.Store.SaveServer(v)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Database error.")
		return
	}
	s.Log.Info("service saved", "kind", kind, "url", v.URL)
	warn := ""
	if len(in.AddProfiles) > 0 {
		if err := s.Profiles.Add(r.Context(), v, in.AddProfiles); err != nil {
			warn = "Saved, but Flixarr's quality profiles couldn't be added: " + err.Error() + "."
		} else if in.DefaultProfile != "" {
			links, _ := s.Store.ProfileLinks(v.ID)
			for _, l := range links {
				if l.Key == in.DefaultProfile {
					for _, p := range s.Profiles.Doc().Profiles {
						if p.ID == l.Key {
							v.QualityProfileID, v.QualityProfileName = l.ArrID, profiles.ArrName(p)
						}
					}
				}
			}
			v, _ = s.Store.SaveServer(v)
		}
	}
	s.Arr.RefreshOne(r.Context(), v)
	sv := s.viewOf(v)
	sv.Warning = warn
	writeJSON(w, http.StatusOK, sv)
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

// serviceProfilesAdd adds Flixarr quality profiles (all, or the keys given) to a connected server.
func (s *Server) serviceProfilesAdd(w http.ResponseWriter, r *http.Request, _ *store.User) {
	v, ok, err := s.Store.FirstServer(r.PathValue("kind"))
	if err != nil || !ok {
		writeErr(w, http.StatusNotFound, "Not connected.")
		return
	}
	var in struct {
		Keys []string `json:"keys"`
	}
	json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in)
	if err := s.Profiles.Add(r.Context(), v, in.Keys); err != nil {
		writeErr(w, http.StatusBadGateway, capitalize(err.Error())+".")
		return
	}
	writeJSON(w, http.StatusOK, s.viewOf(v))
}

// serviceProfileUseFlixarr replaces a profile someone edited in Radarr/Sonarr with Flixarr's current version.
func (s *Server) serviceProfileUseFlixarr(w http.ResponseWriter, r *http.Request, _ *store.User) {
	v, ok, err := s.Store.FirstServer(r.PathValue("kind"))
	if err != nil || !ok {
		writeErr(w, http.StatusNotFound, "Not connected.")
		return
	}
	if err := s.Profiles.UseFlixarrVersion(r.Context(), v, r.PathValue("key")); err != nil {
		writeErr(w, http.StatusBadGateway, capitalize(err.Error())+".")
		return
	}
	writeJSON(w, http.StatusOK, s.viewOf(v))
}

// serviceProfileSkip keeps a profile's current version until the next Flixarr version comes.
func (s *Server) serviceProfileSkip(w http.ResponseWriter, r *http.Request, _ *store.User) {
	v, ok, err := s.Store.FirstServer(r.PathValue("kind"))
	if err != nil || !ok {
		writeErr(w, http.StatusNotFound, "Not connected.")
		return
	}
	if err := s.Profiles.Skip(v, r.PathValue("key")); err != nil {
		writeErr(w, http.StatusBadRequest, capitalize(err.Error())+".")
		return
	}
	writeJSON(w, http.StatusOK, s.viewOf(v))
}

// serviceProfileChanges says what Update would change in a profile (it only reads).
func (s *Server) serviceProfileChanges(w http.ResponseWriter, r *http.Request, _ *store.User) {
	v, ok, err := s.Store.FirstServer(r.PathValue("kind"))
	if err != nil || !ok {
		writeErr(w, http.StatusNotFound, "Not connected.")
		return
	}
	c, err := s.Profiles.Changes(r.Context(), v, r.PathValue("key"))
	if err != nil {
		writeErr(w, http.StatusBadGateway, capitalize(err.Error())+".")
		return
	}
	writeJSON(w, http.StatusOK, c)
}

// profilesInfo lists Flixarr's quality profiles (for the first-setup window, before anything is added).
func (s *Server) profilesInfo(w http.ResponseWriter, r *http.Request, _ *store.User) {
	d := s.Profiles.Doc()
	if d == nil {
		if _, err := s.Profiles.Refresh(r.Context()); err != nil {
			writeErr(w, http.StatusBadGateway, capitalize(err.Error())+".")
			return
		}
		d = s.Profiles.Doc()
	}
	// What a profile does in one app, for Settings → Quality profiles.
	type detail struct {
		BasedOn   string   `json:"basedOn"`
		BasedURL  string   `json:"basedOnUrl"`
		Qualities []string `json:"qualities"` // allowed, best first
		Cutoff    string   `json:"cutoff"`
		Formats   int      `json:"formats"`
		Preferred []string `json:"preferred"` // the highest-scored formats
		Blocked   []string `json:"blocked"`   // formats scored so low they're never grabbed
	}
	type entry struct {
		ID          string             `json:"id"`
		Name        string             `json:"name"`
		Label       string             `json:"label"`
		Description string             `json:"description"`
		ArrName     string             `json:"arrName"`
		Rev         string             `json:"rev"`
		Apps        []string           `json:"apps"`
		Details     map[string]*detail `json:"details"`
	}
	out := []entry{}
	for _, p := range d.Profiles {
		e := entry{ID: p.ID, Name: p.Name, Label: p.Label, Description: p.Description, ArrName: profiles.ArrName(p), Rev: p.Rev, Details: map[string]*detail{}}
		for _, kind := range []string{"radarr", "sonarr"} {
			ap := p.For(kind)
			if ap == nil {
				continue
			}
			e.Apps = append(e.Apps, kind)
			dt := &detail{BasedOn: ap.BasedOn.Name, BasedURL: ap.BasedOn.URL, Cutoff: ap.Profile.Cutoff, Formats: len(ap.Profile.FormatItems), Qualities: []string{}, Preferred: []string{}, Blocked: []string{}}
			for _, it := range ap.Profile.Items {
				if it.Allowed {
					dt.Qualities = append(dt.Qualities, it.Name)
				}
			}
			fi := append([]profiles.FormatItem(nil), ap.Profile.FormatItems...)
			sort.SliceStable(fi, func(i, j int) bool { return fi[i].Score > fi[j].Score })
			for _, f := range fi {
				if f.Score > 0 && len(dt.Preferred) < 6 {
					dt.Preferred = append(dt.Preferred, f.Name)
				}
				if f.Score <= -10000 {
					dt.Blocked = append(dt.Blocked, f.Name)
				}
			}
			e.Details[kind] = dt
		}
		out = append(out, e)
	}
	writeJSON(w, http.StatusOK, map[string]any{"version": d.Version, "source": d.Source, "profiles": out})
}

// serviceRefresh reads one connection's status right away ("Refresh now" on the card).
func (s *Server) serviceRefresh(w http.ResponseWriter, r *http.Request, _ *store.User) {
	v, ok, err := s.Store.FirstServer(r.PathValue("kind"))
	if err != nil || !ok {
		writeErr(w, http.StatusNotFound, "Not connected.")
		return
	}
	s.Arr.RefreshOne(r.Context(), v)
	writeJSON(w, http.StatusOK, s.viewOf(v))
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
		s.Arr.Forget(v)
		s.Log.Info("service removed", "kind", kind)
	}
	w.WriteHeader(http.StatusNoContent)
}
