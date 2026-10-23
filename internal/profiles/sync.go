package profiles

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Flixarr/flixarr/internal/arr"
	"github.com/Flixarr/flixarr/internal/store"
	"github.com/Flixarr/flixarr/internal/testinstall"
)

const keyDoc = "profiles_doc" // the last profiles document downloaded (JSON)

// Manager downloads the profiles and adds / updates them on Radarr and Sonarr servers.
//
// Rules it keeps:
//   - It never changes or deletes anything it didn't create. Custom formats already on a server are
//     reused only when they match Flixarr's exactly; otherwise Flixarr adds its own copy named "<name> (Flixarr)".
//   - Newer versions are downloaded on their own but only applied when an admin presses Update (#94);
//     adding a profile for the first time applies it right away.
//   - A Flixarr profile someone edited in Radarr/Sonarr is left alone until an admin replaces it.
//   - A Flixarr profile someone deleted in Radarr/Sonarr isn't put back on its own.
type Manager struct {
	Store     *store.Store
	API       string // e.g. https://api.flixarr.app
	UserAgent string
	HTTP      *http.Client
	Log       *slog.Logger

	mu  sync.Mutex
	doc *Doc
	run sync.Mutex // one push at a time
}

// Doc returns the current profiles document (the last one downloaded), or nil before the first download.
func (m *Manager) Doc() *Doc {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.doc == nil {
		var d Doc
		if json.Unmarshal([]byte(m.Store.Setting(keyDoc, "")), &d) == nil && d.Version != "" {
			m.doc = &d
		}
	}
	return m.doc
}

// Refresh downloads the profiles from the API. It reports whether the version changed.
func (m *Manager) Refresh(ctx context.Context) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(m.API, "/")+"/v1/profiles", nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("User-Agent", m.UserAgent)
	hc := m.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	res, err := hc.Do(req)
	if err != nil {
		return false, fmt.Errorf("couldn't reach the Flixarr API: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return false, fmt.Errorf("the Flixarr API answered %s", res.Status)
	}
	var d Doc
	if err := json.NewDecoder(res.Body).Decode(&d); err != nil || d.Version == "" {
		return false, errors.New("the Flixarr API sent profiles Flixarr can't read")
	}
	old := m.Doc()
	changed := old == nil || old.Version != d.Version
	if changed {
		b, _ := json.Marshal(d)
		if err := m.Store.SetSetting(keyDoc, string(b)); err != nil {
			return false, err
		}
	}
	m.mu.Lock()
	m.doc = &d
	m.mu.Unlock()
	return changed, nil
}

func (m *Manager) docOrFetch(ctx context.Context) (*Doc, error) {
	if d := m.Doc(); d != nil {
		return d, nil
	}
	if _, err := m.Refresh(ctx); err != nil {
		return nil, err
	}
	return m.Doc(), nil
}

func client(v store.Server) *arr.Client {
	return &arr.Client{Kind: v.Kind, URL: v.URL, APIKey: v.APIKey}
}

// ---------- status ----------

// ProfileStatus is one Flixarr profile as it stands on one server.
type ProfileStatus struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Label       string `json:"label"`
	Description string `json:"description"`
	ArrName     string `json:"arrName"`
	Installed   bool   `json:"installed"`
	ArrID       int    `json:"arrId,omitempty"`
	// State: "" (not added), "ok" (up to date), "update" (a newer version is waiting for an admin),
	// "customized" (edited in Radarr/Sonarr; a newer version is waiting if UpdateAvailable), "missing" (deleted in Radarr/Sonarr).
	State           string `json:"state"`
	UpdateAvailable bool   `json:"updateAvailable"`
	Skipped         bool   `json:"skipped"` // the newer version was skipped; offered again when the next one comes
	Rev             string `json:"rev,omitempty"`
	UpdatedAt       string `json:"updatedAt,omitempty"`
}

// Status lists every Flixarr profile for a server, added or not.
func (m *Manager) Status(v store.Server) ([]ProfileStatus, error) {
	d := m.Doc()
	if d == nil {
		return []ProfileStatus{}, nil
	}
	links, err := m.Store.ProfileLinks(v.ID)
	if err != nil {
		return nil, err
	}
	byKey := map[string]store.ProfileLink{}
	for _, l := range links {
		byKey[l.Key] = l
	}
	out := []ProfileStatus{}
	for _, p := range d.Profiles {
		if p.For(v.Kind) == nil {
			continue
		}
		s := ProfileStatus{Key: p.ID, Name: p.Name, Label: p.Label, Description: p.Description, ArrName: ArrName(p)}
		if l, ok := byKey[p.ID]; ok {
			s.Installed, s.ArrID, s.State, s.UpdatedAt = l.State != "missing", l.ArrID, l.State, l.UpdatedAt
			s.Rev = l.Rev
			s.Skipped = l.Rev != p.Rev && m.skipped(v.ID, p.ID) == p.Rev
			s.UpdateAvailable = l.Rev != p.Rev && !s.Skipped
			if s.State == "ok" && s.UpdateAvailable {
				s.State = "update"
			}
		}
		out = append(out, s)
	}
	return out, nil
}

// ---------- add / update ----------

// server is what one push needs from a Radarr/Sonarr server, read once.
type server struct {
	v         store.Server
	c         *arr.Client
	schema    arrProfile
	formats   []arrFormat
	profiles  []arrProfile
	languages []arrLanguage
}

func (m *Manager) load(ctx context.Context, v store.Server) (*server, error) {
	s := &server{v: v, c: client(v)}
	if err := s.c.Get(ctx, "qualityprofile/schema", &s.schema); err != nil {
		return nil, err
	}
	if err := s.c.Get(ctx, "customformat", &s.formats); err != nil {
		return nil, err
	}
	if err := s.c.Get(ctx, "qualityprofile", &s.profiles); err != nil {
		return nil, err
	}
	if v.Kind == "radarr" {
		s.c.Get(ctx, "language", &s.languages) // optional; the schema's default language is used otherwise
	}
	return s, nil
}

// ensureFormats makes sure every custom format the profile needs is on the server and returns their IDs
// by Flixarr name. Formats Flixarr created are updated if their rules changed; formats that were already
// there are only reused when identical, never changed.
func (m *Manager) ensureFormats(ctx context.Context, s *server, d *Doc, b Body) (map[string]int, error) {
	links, err := m.Store.FormatLinks(s.v.ID)
	if err != nil {
		return nil, err
	}
	live := map[int]arrFormat{}
	byName := map[string]arrFormat{}
	for _, f := range s.formats {
		live[f.ID] = f
		byName[f.Name] = f
	}
	ids := map[string]int{}
	lib := d.CustomFormats[s.v.Kind]
	for _, fi := range b.FormatItems {
		want, ok := lib[fi.Name]
		if !ok {
			return nil, fmt.Errorf("the profiles are missing custom format %q", fi.Name)
		}
		h := specHash(want.Specifications)
		if l, ok := links[fi.Name]; ok {
			if cur, still := live[l.ArrID]; still {
				if l.Owned && specHash(cur.Specifications) != h {
					body := arrFormat{ID: cur.ID, Name: cur.Name, IncludeCustomFormatWhenRenaming: want.IncludeCustomFormatWhenRenaming, Specifications: want.Specifications}
					if err := s.c.Put(ctx, fmt.Sprintf("customformat/%d", cur.ID), body, nil); err != nil {
						return nil, fmt.Errorf("couldn't update custom format %q: %w", cur.Name, err)
					}
					m.Store.SaveFormatLink(store.FormatLink{ServerID: s.v.ID, Name: fi.Name, ArrID: cur.ID, Owned: true, SpecHash: h})
				}
				ids[fi.Name] = l.ArrID
				continue
			}
			m.Store.DeleteFormatLink(s.v.ID, fi.Name) // deleted in Radarr/Sonarr: look again below
		}
		// Already there with the same rules: reuse it (and never change it).
		if f, ok := byName[fi.Name]; ok && specHash(f.Specifications) == h {
			ids[fi.Name] = f.ID
			m.Store.SaveFormatLink(store.FormatLink{ServerID: s.v.ID, Name: fi.Name, ArrID: f.ID, Owned: false, SpecHash: h})
			continue
		}
		// Same name but different rules (someone's own version): Flixarr adds its own copy beside it.
		name := fi.Name
		if _, taken := byName[name]; taken {
			name = fi.Name + " (Flixarr)"
		}
		var created arrFormat
		if f, ok := byName[name]; ok { // a copy from an earlier install of Flixarr
			body := arrFormat{ID: f.ID, Name: name, IncludeCustomFormatWhenRenaming: want.IncludeCustomFormatWhenRenaming, Specifications: want.Specifications}
			if err := s.c.Put(ctx, fmt.Sprintf("customformat/%d", f.ID), body, nil); err != nil {
				return nil, fmt.Errorf("couldn't update custom format %q: %w", name, err)
			}
			created = f
		} else {
			body := arrFormat{Name: name, IncludeCustomFormatWhenRenaming: want.IncludeCustomFormatWhenRenaming, Specifications: want.Specifications}
			if err := s.c.Post(ctx, "customformat", body, &created); err != nil {
				return nil, fmt.Errorf("couldn't add custom format %q: %w", name, err)
			}
			created.Name = name
			s.formats = append(s.formats, created)
			byName[name] = created
		}
		ids[fi.Name] = created.ID
		m.Store.SaveFormatLink(store.FormatLink{ServerID: s.v.ID, Name: fi.Name, ArrID: created.ID, Owned: true, SpecHash: h})
	}
	return ids, nil
}

// push adds or replaces one Flixarr profile on the server and records it.
func (m *Manager) push(ctx context.Context, s *server, d *Doc, p Profile, arrID int) error {
	if testinstall.On {
		return errors.New("quality profiles aren't pushed from a dev install: it shares Radarr and Sonarr with the real one")
	}
	ap := p.For(s.v.Kind)
	if ap == nil {
		return fmt.Errorf("%s has no %s version", p.Label, arr.Kinds[s.v.Kind])
	}
	ids, err := m.ensureFormats(ctx, s, d, ap.Profile)
	if err != nil {
		return err
	}
	// Formats may have been added, so read the full list again: every profile scores every format.
	if err := s.c.Get(ctx, "customformat", &s.formats); err != nil {
		return err
	}
	body, err := buildProfile(ArrName(p), ap.Profile, s.schema, ids, s.formats, s.languages)
	if err != nil {
		return fmt.Errorf("%s: %w", p.Label, err)
	}
	var saved arrProfile
	if arrID == 0 {
		// A profile with this name from an earlier install of Flixarr is taken over rather than duplicated.
		for _, x := range s.profiles {
			if x.Name == body.Name {
				arrID = x.ID
			}
		}
	}
	if arrID == 0 {
		err = s.c.Post(ctx, "qualityprofile", body, &saved)
	} else {
		body.ID = arrID
		err = s.c.Put(ctx, fmt.Sprintf("qualityprofile/%d", arrID), body, &saved)
	}
	if err != nil {
		return fmt.Errorf("couldn't save %s in %s: %w", body.Name, arr.Kinds[s.v.Kind], err)
	}
	// Read it back as Radarr/Sonarr stored it, so later edits there can be spotted.
	if err := s.c.Get(ctx, fmt.Sprintf("qualityprofile/%d", saved.ID), &saved); err != nil {
		return err
	}
	return m.Store.SaveProfileLink(store.ProfileLink{ServerID: s.v.ID, Key: p.ID, ArrID: saved.ID, Rev: p.Rev, PushedHash: fingerprint(saved), State: "ok"}, true)
}

// Add puts the given Flixarr profiles (by key; all when empty) on a server. Profiles already there are
// brought up to date unless someone edited them.
func (m *Manager) Add(ctx context.Context, v store.Server, keys []string) error {
	m.run.Lock()
	defer m.run.Unlock()
	d, err := m.docOrFetch(ctx)
	if err != nil {
		return err
	}
	s, err := m.load(ctx, v)
	if err != nil {
		return err
	}
	links, _ := m.Store.ProfileLinks(v.ID)
	have := map[string]store.ProfileLink{}
	for _, l := range links {
		have[l.Key] = l
	}
	want := map[string]bool{}
	for _, k := range keys {
		want[k] = true
	}
	for _, p := range d.Profiles {
		if (len(keys) > 0 && !want[p.ID]) || p.For(v.Kind) == nil {
			continue
		}
		id := 0
		if l, ok := have[p.ID]; ok && l.State != "missing" {
			id = l.ArrID
		}
		if err := m.push(ctx, s, d, p, id); err != nil {
			return err
		}
		m.Log.Info("quality profile added", "server", v.Name, "profile", p.ID, "rev", p.Rev)
	}
	return nil
}

// UseFlixarrVersion puts Flixarr's current version of a profile on a server (Update in Settings), replacing
// any edits made there.
func (m *Manager) UseFlixarrVersion(ctx context.Context, v store.Server, key string) error {
	m.run.Lock()
	defer m.run.Unlock()
	d, err := m.docOrFetch(ctx)
	if err != nil {
		return err
	}
	var p *Profile
	for i := range d.Profiles {
		if d.Profiles[i].ID == key {
			p = &d.Profiles[i]
		}
	}
	if p == nil {
		return fmt.Errorf("there's no Flixarr profile %q", key)
	}
	links, _ := m.Store.ProfileLinks(v.ID)
	id := 0
	for _, l := range links {
		if l.Key == key && l.State != "missing" {
			id = l.ArrID
		}
	}
	s, err := m.load(ctx, v)
	if err != nil {
		return err
	}
	if err := m.push(ctx, s, d, *p, id); err != nil {
		return err
	}
	m.Store.SetSetting(skipKey(v.ID, key), "")
	go m.UpdateTodo()
	return nil
}

// Sync checks every Flixarr profile on a server: edited ones are marked customized, deleted ones missing.
// Nothing is changed in Radarr/Sonarr: newer versions wait for an admin (Status shows them as updates).
func (m *Manager) Sync(ctx context.Context, v store.Server) (int, error) {
	m.run.Lock()
	defer m.run.Unlock()
	d := m.Doc()
	links, err := m.Store.ProfileLinks(v.ID)
	if err != nil || d == nil || len(links) == 0 {
		return 0, err
	}
	s, err := m.load(ctx, v)
	if err != nil {
		return 0, err
	}
	live := map[int]arrProfile{}
	for _, p := range s.profiles {
		live[p.ID] = p
	}
	for _, l := range links {
		cur, still := live[l.ArrID]
		switch {
		case !still:
			l.State = "missing"
		case fingerprint(cur) != l.PushedHash:
			l.State = "customized"
		default:
			l.State = "ok"
		}
		m.Store.SaveProfileLink(l, false)
	}
	return 0, nil
}

// SyncAll downloads the profiles and syncs every server; it's the "Quality profiles: sync" job.
func (m *Manager) SyncAll(ctx context.Context) (string, error) {
	changed, err := m.Refresh(ctx)
	if err != nil && m.Doc() == nil {
		return "", err
	}
	servers, err := m.Store.Servers()
	if err != nil {
		return "", err
	}
	failed := 0
	for _, v := range servers {
		if _, err := m.Sync(ctx, v); err != nil {
			failed++
			m.Log.Warn("quality profile sync", "server", v.Name, "err", err)
		}
	}
	waiting := m.UpdateTodo()
	msg := fmt.Sprintf("Profiles %s, %d update%s waiting", m.Doc().Version, waiting, map[bool]string{true: "", false: "s"}[waiting == 1])
	if changed {
		msg += " (new version)"
	}
	if failed > 0 {
		return msg, fmt.Errorf("%d server(s) couldn't be synced", failed)
	}
	return msg, nil
}
