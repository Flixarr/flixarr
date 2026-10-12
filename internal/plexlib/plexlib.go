// Package plexlib connects Flixarr to a Plex Media Server and keeps a list of what's in its movie and
// TV libraries (by TMDB ID), so titles show as available even if nobody requested them.
package plexlib

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Flixarr/flixarr/internal/store"
)

// Settings keys.
const (
	keyAdminToken = "plex_admin_token"  // the admin's plex.tv token (encrypted), from sign-in
	keyServerID   = "plex_server_id"    // machine identifier
	keyServerName = "plex_server_name"  //
	keyServerURL  = "plex_server_url"   // the connection Flixarr uses
	keyToken      = "plex_server_token" // the server's access token (encrypted)
	keyLibraries  = "plex_libraries"    // JSON list of section keys to sync
	keyLastFull   = "plex_last_full"    // RFC3339
)

// Library keeps the Plex connection and syncs its items.
type Library struct {
	Store     *store.Store
	ClientID  string
	Version   string
	Log       *slog.Logger
	HTTP      *http.Client
	TVBaseURL string // plex.tv, for tests
}

func (l *Library) client() *http.Client {
	if l.HTTP != nil {
		return l.HTTP
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func (l *Library) get(ctx context.Context, base, path, token string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Plex-Token", token)
	req.Header.Set("X-Plex-Product", "Flixarr")
	req.Header.Set("X-Plex-Version", l.Version)
	req.Header.Set("X-Plex-Client-Identifier", l.ClientID)
	res, err := l.client().Do(req)
	if err != nil {
		return errors.New("couldn't reach it")
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusUnauthorized {
		return errors.New("Plex refused the sign-in; sign in with Plex again")
	}
	if res.StatusCode >= 300 {
		return fmt.Errorf("Plex answered %s", res.Status)
	}
	return json.NewDecoder(res.Body).Decode(out)
}

// ---------- servers on the admin's account ----------

// Connection is one way to reach a server.
type Connection struct {
	URI   string `json:"uri"`
	Local bool   `json:"local"`
	Relay bool   `json:"relay"`
}

// Server is a Plex Media Server the admin's account can use.
type Server struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Owned       bool         `json:"owned"`
	Connections []Connection `json:"connections"`
	token       string
}

// Servers lists the Plex servers on the admin's account.
func (l *Library) Servers(ctx context.Context) ([]Server, error) {
	tok := l.Store.SecretSetting(keyAdminToken, "")
	if tok == "" {
		return nil, errors.New("sign in with Plex again so Flixarr can see your servers")
	}
	base := l.TVBaseURL
	if base == "" {
		base = "https://plex.tv"
	}
	var res []struct {
		Name             string       `json:"name"`
		ClientIdentifier string       `json:"clientIdentifier"`
		Provides         string       `json:"provides"`
		Owned            bool         `json:"owned"`
		AccessToken      string       `json:"accessToken"`
		Connections      []Connection `json:"connections"`
	}
	if err := l.get(ctx, base, "/api/v2/resources?includeHttps=1&includeRelay=1", tok, &res); err != nil {
		return nil, fmt.Errorf("plex.tv: %w", err)
	}
	out := []Server{}
	for _, r := range res {
		if !strings.Contains(r.Provides, "server") {
			continue
		}
		cs := append([]Connection(nil), r.Connections...)
		// Local first, relay last: the order Flixarr tries them in.
		sort.SliceStable(cs, func(i, j int) bool { return rank(cs[i]) < rank(cs[j]) })
		out = append(out, Server{ID: r.ClientIdentifier, Name: r.Name, Owned: r.Owned, Connections: cs, token: r.AccessToken})
	}
	return out, nil
}

func rank(c Connection) int {
	switch {
	case c.Relay:
		return 2
	case c.Local:
		return 0
	}
	return 1
}

// Connect picks a server: it tries the server's connections (local first) and keeps the first that answers.
func (l *Library) Connect(ctx context.Context, id string) (Status, error) {
	servers, err := l.Servers(ctx)
	if err != nil {
		return Status{}, err
	}
	for _, s := range servers {
		if s.ID != id {
			continue
		}
		for _, c := range s.Connections {
			cctx, cancel := context.WithTimeout(ctx, 6*time.Second)
			var ident struct {
				MediaContainer struct {
					MachineIdentifier string `json:"machineIdentifier"`
				} `json:"MediaContainer"`
			}
			err := l.get(cctx, c.URI, "/identity", s.token, &ident)
			cancel()
			if err == nil && ident.MediaContainer.MachineIdentifier == s.ID {
				l.Store.SetSetting(keyServerID, s.ID)
				l.Store.SetSetting(keyServerName, s.Name)
				l.Store.SetSetting(keyServerURL, c.URI)
				l.Store.SetSecretSetting(keyToken, s.token)
				l.Log.Info("plex server connected", "name", s.Name, "via", c.URI)
				return l.Status(ctx)
			}
		}
		return Status{}, fmt.Errorf("couldn't reach %s from Flixarr on any of its addresses", s.Name)
	}
	return Status{}, errors.New("that server isn't on your Plex account")
}

// Disconnect forgets the server and its items.
func (l *Library) Disconnect() error {
	for _, k := range []string{keyServerID, keyServerName, keyServerURL, keyToken, keyLibraries, keyLastFull} {
		l.Store.SetSetting(k, "")
	}
	return l.Store.ClearPlexItems()
}

// ---------- libraries ----------

// Section is a movie or TV library on the server.
type Section struct {
	Key     string `json:"key"`
	Type    string `json:"type"` // movie | show
	Title   string `json:"title"`
	Enabled bool   `json:"enabled"`
	Items   int    `json:"items"`
}

// Status is the Plex connection as Settings shows it.
type Status struct {
	Connected bool      `json:"connected"`
	ID        string    `json:"id,omitempty"`
	Name      string    `json:"name,omitempty"`
	URL       string    `json:"url,omitempty"`
	Sections  []Section `json:"sections"`
	LastFull  string    `json:"lastFull,omitempty"`
	Error     string    `json:"error,omitempty"`
}

func (l *Library) conn() (base, token string, ok bool) {
	base, token = l.Store.Setting(keyServerURL, ""), l.Store.SecretSetting(keyToken, "")
	return base, token, base != "" && token != ""
}

func (l *Library) enabled() map[string]bool {
	var keys []string
	json.Unmarshal([]byte(l.Store.Setting(keyLibraries, "")), &keys)
	if keys == nil {
		return nil // never chosen: every movie and TV library
	}
	m := map[string]bool{}
	for _, k := range keys {
		m[k] = true
	}
	return m
}

// Sections lists the server's movie and TV libraries, marking the ones Flixarr syncs.
func (l *Library) Sections(ctx context.Context) ([]Section, error) {
	base, tok, ok := l.conn()
	if !ok {
		return nil, errors.New("no Plex server connected")
	}
	var res struct {
		MediaContainer struct {
			Directory []struct {
				Key   string `json:"key"`
				Type  string `json:"type"`
				Title string `json:"title"`
				Agent string `json:"agent"`
			} `json:"Directory"`
		} `json:"MediaContainer"`
	}
	if err := l.get(ctx, base, "/library/sections", tok, &res); err != nil {
		return nil, err
	}
	on := l.enabled()
	counts, _ := l.Store.PlexItemCounts()
	out := []Section{}
	for _, d := range res.MediaContainer.Directory {
		if d.Type != "movie" && d.Type != "show" {
			continue
		}
		// Libraries without a metadata agent (home videos) have no TMDB IDs: off unless chosen.
		def := d.Agent != "com.plexapp.agents.none"
		out = append(out, Section{Key: d.Key, Type: d.Type, Title: d.Title, Enabled: (on == nil && def) || on[d.Key], Items: counts[d.Key]})
	}
	return out, nil
}

// SetSections chooses which libraries to sync.
func (l *Library) SetSections(keys []string) error {
	b, _ := json.Marshal(keys)
	return l.Store.SetSetting(keyLibraries, string(b))
}

// Status reports the connection and libraries.
func (l *Library) Status(ctx context.Context) (Status, error) {
	st := Status{Sections: []Section{}}
	if _, _, ok := l.conn(); !ok {
		return st, nil
	}
	st.Connected, st.ID, st.Name, st.URL, st.LastFull = true, l.Store.Setting(keyServerID, ""), l.Store.Setting(keyServerName, ""), l.Store.Setting(keyServerURL, ""), l.Store.Setting(keyLastFull, "")
	secs, err := l.Sections(ctx)
	if err != nil {
		st.Error = err.Error()
	} else {
		st.Sections = secs
	}
	return st, nil
}

// ---------- syncing items ----------

type metadata struct {
	RatingKey string `json:"ratingKey"`
	Type      string `json:"type"`
	Title     string `json:"title"`
	AddedAt   int64  `json:"addedAt"`
	PlexGUID  string `json:"guid"` // Plex's own ID; declared so it isn't mistaken for Guid below (json matches names case-insensitively)
	Guid      []struct {
		ID string `json:"id"`
	} `json:"Guid"`
}

func (m metadata) tmdb() int {
	for _, g := range m.Guid {
		if strings.HasPrefix(g.ID, "tmdb://") {
			n, _ := strconv.Atoi(strings.TrimPrefix(g.ID, "tmdb://"))
			return n
		}
	}
	return 0
}

// stamp is a sortable scan time; nanoseconds so two scans in the same second still prune correctly.
func stamp() string { return time.Now().UTC().Format("2006-01-02T15:04:05.000000000Z") }

func kindOf(plexType string) string {
	if plexType == "show" {
		return "series"
	}
	return "movie"
}

// FullScan reads every item in the chosen libraries and replaces the list; it's the
// "Plex: full library scan" job.
func (l *Library) FullScan(ctx context.Context) (string, error) {
	base, tok, ok := l.conn()
	if !ok {
		return "No Plex server connected", nil
	}
	secs, err := l.Sections(ctx)
	if err != nil {
		return "", err
	}
	started := stamp()
	total := 0
	for _, sec := range secs {
		if !sec.Enabled {
			continue
		}
		for start := 0; ; start += 500 {
			var res struct {
				MediaContainer struct {
					TotalSize int        `json:"totalSize"`
					Metadata  []metadata `json:"Metadata"`
				} `json:"MediaContainer"`
			}
			path := fmt.Sprintf("/library/sections/%s/all?includeGuids=1&X-Plex-Container-Start=%d&X-Plex-Container-Size=500", url.PathEscape(sec.Key), start)
			if err := l.get(ctx, base, path, tok, &res); err != nil {
				return "", fmt.Errorf("%s: %w", sec.Title, err)
			}
			for _, m := range res.MediaContainer.Metadata {
				if id := m.tmdb(); id > 0 {
					l.Store.SavePlexItem(store.PlexItem{Type: kindOf(sec.Type), TmdbID: id, RatingKey: m.RatingKey, Section: sec.Key, Title: m.Title, AddedAt: m.AddedAt, SeenAt: started})
					total++
				}
			}
			if len(res.MediaContainer.Metadata) == 0 || start+500 >= res.MediaContainer.TotalSize {
				break
			}
		}
	}
	// Anything not seen in this scan was removed from Plex (or its library was switched off).
	removed, err := l.Store.PrunePlexItems(started)
	if err != nil {
		return "", err
	}
	l.Store.SetSetting(keyLastFull, time.Now().UTC().Format(time.RFC3339))
	return fmt.Sprintf("%d titles in Plex (%d removed)", total, removed), nil
}

// RecentScan reads the newest additions to the chosen libraries; it's the "Plex: recently added" job.
func (l *Library) RecentScan(ctx context.Context) (string, error) {
	base, tok, ok := l.conn()
	if !ok {
		return "No Plex server connected", nil
	}
	secs, err := l.Sections(ctx)
	if err != nil {
		return "", err
	}
	now := stamp()
	added := 0
	for _, sec := range secs {
		if !sec.Enabled {
			continue
		}
		var res struct {
			MediaContainer struct {
				Metadata []metadata `json:"Metadata"`
			} `json:"MediaContainer"`
		}
		// Newest first. For TV that's when the show itself arrived; later episodes don't matter here.
		path := fmt.Sprintf("/library/sections/%s/all?includeGuids=1&sort=addedAt:desc&X-Plex-Container-Start=0&X-Plex-Container-Size=50", url.PathEscape(sec.Key))
		if err := l.get(ctx, base, path, tok, &res); err != nil {
			return "", fmt.Errorf("%s: %w", sec.Title, err)
		}
		for _, m := range res.MediaContainer.Metadata {
			if id := m.tmdb(); id > 0 {
				if isNew, _ := l.Store.SavePlexItem(store.PlexItem{Type: kindOf(sec.Type), TmdbID: id, RatingKey: m.RatingKey, Section: sec.Key, Title: m.Title, AddedAt: m.AddedAt, SeenAt: now}); isNew {
					added++
				}
			}
		}
	}
	return fmt.Sprintf("%d new in Plex", added), nil
}

// ---------- lookups ----------

// Item is where a title is in Plex.
type Item struct {
	RatingKey string `json:"ratingKey"`
	URL       string `json:"url"` // opens it in Plex Web
}

// Lookup says which of the given titles ("movie:603", "series:1396") are in Plex.
func (l *Library) Lookup(keys []string) (map[string]Item, error) {
	items, err := l.Store.PlexItemsFor(keys)
	if err != nil {
		return nil, err
	}
	machine := l.Store.Setting(keyServerID, "")
	out := map[string]Item{}
	for k, it := range items {
		out[k] = Item{RatingKey: it.RatingKey, URL: fmt.Sprintf("https://app.plex.tv/desktop/#!/server/%s/details?key=%s", machine, url.QueryEscape("/library/metadata/"+it.RatingKey))}
	}
	return out, nil
}
