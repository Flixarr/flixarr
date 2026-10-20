// Package plexlib connects Flixarr to a Plex Media Server and keeps a list of what's in its movie and
// TV libraries (by TMDB ID), so titles show as available even if nobody requested them.
package plexlib

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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
	keyWebURL     = "plex_web_url"      // Play on Plex / Open in Plex go here; blank = app.plex.tv
	keyAddrPicked = "plex_address_set"  // "true" when an admin picked or typed the address (#114); else Automatic
)

var errNotFound = errors.New("Plex answered 404 Not Found")

// Library keeps the Plex connection and syncs its items.
type Library struct {
	Store     *store.Store
	ClientID  string
	Version   string
	Log       *slog.Logger
	HTTP      *http.Client
	TVBaseURL string // plex.tv, for tests

	// OnAdded is told about titles that just showed up in Plex ("movie:603" keys).
	OnAdded func(keys []string)
	// OnRemoved is told about titles that left Plex; episodes says episodes or seasons of some show did
	// (Plex doesn't say which show).
	OnRemoved func(keys []string, episodes bool)
	live      atomic.Bool
	lastFull  atomic.Int64 // unix time of the last full scan
	fullDue   atomic.Bool  // a full read is scheduled
	// relisten drops the live connection so it comes back on a new address.
	relisten   context.CancelFunc
	relistenMu sync.Mutex
}

func (l *Library) client() *http.Client {
	if l.HTTP != nil {
		return l.HTTP
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func (l *Library) get(ctx context.Context, base, path, token string, out any) error {
	return l.do(ctx, http.MethodGet, base, path, token, out)
}

func (l *Library) do(ctx context.Context, method, base, path, token string, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(base, "/")+path, nil)
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
	if res.StatusCode == http.StatusNotFound {
		return errNotFound
	}
	if res.StatusCode >= 300 {
		return fmt.Errorf("Plex answered %s", res.Status)
	}
	if err := json.NewDecoder(res.Body).Decode(out); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil // some answers (a scan request) have no body
}

// ---------- servers on the admin's account ----------

// Connection is one way to reach a server.
type Connection struct {
	URI     string `json:"uri"`
	Local   bool   `json:"local"`
	Relay   bool   `json:"relay"`
	Address string `json:"address,omitempty"` // the IP (plex.tv sends it with each connection)
	Port    int    `json:"port,omitempty"`
}

// Server is a Plex Media Server the admin's account can use.
type Server struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Owned       bool         `json:"owned"`
	Connections []Connection `json:"connections"`
	token       string
}

// Servers lists the Plex servers the admin's account owns.
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
		if !strings.Contains(r.Provides, "server") || !r.Owned {
			continue // only servers you own: a shared server's library isn't yours to manage
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

// Connect picks a server and the fastest of its addresses that answers (Plex's relay only when nothing
// else does).
func (l *Library) Connect(ctx context.Context, id string) (Status, error) {
	servers, err := l.Servers(ctx)
	if err != nil {
		return Status{}, err
	}
	for _, s := range servers {
		if s.ID != id {
			continue
		}
		best := fastest(l.probeAll(ctx, candidates(s.Connections), s.token, s.ID))
		if best == "" {
			return Status{}, fmt.Errorf("couldn't reach %s from Flixarr on any of its addresses", s.Name)
		}
		l.Store.SetSetting(keyServerID, s.ID)
		l.Store.SetSetting(keyServerName, s.Name)
		l.Store.SetSetting(keyServerURL, best)
		l.Store.SetSetting(keyAddrPicked, "")
		l.Store.SetSecretSetting(keyToken, s.token)
		l.Log.Info("plex server connected", "name", s.Name, "via", best)
		return l.Status(ctx)
	}
	return Status{}, errors.New("that server isn't on your Plex account")
}

// ---------- which address (#114) ----------

// Address is one way to reach the connected server, as Settings lists them, with how fast it answered.
type Address struct {
	URI   string `json:"uri"`
	Kind  string `json:"kind"`            // local | remote | relay | yours (typed by an admin)
	MS    int    `json:"ms"`              // how long /identity took; 0 when it didn't answer
	Error string `json:"error,omitempty"` // why it didn't
}

// candidates are a server's addresses: plex.tv's, plus the plain http://IP:port of each local one (plex.tv
// only lists the https plex.direct name, which some networks can't resolve).
func candidates(conns []Connection) []Address {
	out := []Address{}
	seen := map[string]bool{}
	add := func(uri, kind string) {
		if uri != "" && !seen[uri] {
			seen[uri] = true
			out = append(out, Address{URI: uri, Kind: kind})
		}
	}
	for _, c := range conns {
		kind := "remote"
		if c.Relay {
			kind = "relay"
		} else if c.Local {
			kind = "local"
		}
		add(c.URI, kind)
		if c.Local && !c.Relay && c.Address != "" && c.Port > 0 {
			add(fmt.Sprintf("http://%s:%d", c.Address, c.Port), "local")
		}
	}
	return out
}

// probe times /identity on an address, checking it's the right server.
func (l *Library) probe(ctx context.Context, uri, token, id string) (time.Duration, error) {
	cctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	var ident struct {
		MediaContainer struct {
			MachineIdentifier string `json:"machineIdentifier"`
		} `json:"MediaContainer"`
	}
	start := time.Now()
	if err := l.get(cctx, uri, "/identity", token, &ident); err != nil {
		return 0, err
	}
	if ident.MediaContainer.MachineIdentifier != id {
		return 0, errors.New("a different Plex server answered there")
	}
	return max(time.Since(start), time.Millisecond), nil
}

// probeAll tries every address at once.
func (l *Library) probeAll(ctx context.Context, list []Address, token, id string) []Address {
	var wg sync.WaitGroup
	for i := range list {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d, err := l.probe(ctx, list[i].URI, token, id)
			if err != nil {
				list[i].Error = err.Error()
				return
			}
			list[i].MS = int(d.Milliseconds())
			if list[i].MS == 0 {
				list[i].MS = 1
			}
		}()
	}
	wg.Wait()
	return list
}

// fastest is the quickest address that answered; the relay only when nothing else did.
func fastest(list []Address) string {
	best, bestMS := "", 0
	for _, relay := range []bool{false, true} {
		for _, a := range list {
			if a.MS > 0 && (a.Kind == "relay") == relay && (best == "" || a.MS < bestMS) {
				best, bestMS = a.URI, a.MS
			}
		}
		if best != "" {
			return best
		}
	}
	return ""
}

// Addresses lists the connected server's addresses, each tried now, and whether Flixarr picks on its own.
func (l *Library) Addresses(ctx context.Context) (list []Address, auto bool, err error) {
	id := l.Store.Setting(keyServerID, "")
	_, token, ok := l.conn()
	if !ok {
		return nil, false, errors.New("connect a Plex server first")
	}
	servers, err := l.Servers(ctx)
	if err != nil {
		return nil, false, err
	}
	for _, s := range servers {
		if s.ID == id {
			list = candidates(s.Connections)
		}
	}
	if cur := l.Store.Setting(keyServerURL, ""); !slices.ContainsFunc(list, func(a Address) bool { return a.URI == cur }) {
		list = append(list, Address{URI: cur, Kind: "yours"})
	}
	return l.probeAll(ctx, list, token, id), l.Store.Setting(keyAddrPicked, "") != "true", nil
}

// SetAddress makes Flixarr use an address ("" = Automatic: the fastest now). It has to answer as the
// connected server.
func (l *Library) SetAddress(ctx context.Context, uri string) (Status, error) {
	id := l.Store.Setting(keyServerID, "")
	_, token, ok := l.conn()
	if !ok {
		return Status{}, errors.New("connect a Plex server first")
	}
	uri = strings.TrimRight(strings.TrimSpace(uri), "/")
	if uri == "" {
		list, _, err := l.Addresses(ctx)
		if err != nil {
			return Status{}, err
		}
		if uri = fastest(list); uri == "" {
			return Status{}, errors.New("none of the server's addresses answered")
		}
		l.Store.SetSetting(keyAddrPicked, "")
	} else {
		u, err := url.Parse(uri)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return Status{}, errors.New("that address doesn't look right; it should be like http://192.168.1.10:32400")
		}
		if _, err := l.probe(ctx, uri, token, id); err != nil {
			return Status{}, fmt.Errorf("the server didn't answer there: %w", err)
		}
		l.Store.SetSetting(keyAddrPicked, "true")
	}
	l.Store.SetSetting(keyServerURL, uri)
	l.Log.Info("plex address set", "via", uri)
	l.relistenMu.Lock()
	if l.relisten != nil {
		l.relisten()
	}
	l.relistenMu.Unlock()
	return l.Status(ctx)
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
	WebURL    string    `json:"webUrl"` // blank = app.plex.tv
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
	st := Status{Sections: []Section{}, WebURL: l.Store.Setting(keyWebURL, "")}
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
	DeletedAt int64  `json:"deletedAt"` // in Plex's trash: its files are gone, so it isn't playable
	PlexGUID  string `json:"guid"`      // Plex's own ID; declared so it isn't mistaken for Guid below (json matches names case-insensitively)
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
				if id := m.tmdb(); id > 0 && m.DeletedAt == 0 {
					l.Store.SavePlexItem(store.PlexItem{Type: kindOf(sec.Type), TmdbID: id, RatingKey: m.RatingKey, Section: sec.Key, Title: m.Title, AddedAt: m.AddedAt, SeenAt: started})
					total++
				}
			}
			if len(res.MediaContainer.Metadata) == 0 || start+500 >= res.MediaContainer.TotalSize {
				break
			}
		}
	}
	// Anything not seen in this scan was removed from Plex, is in its trash, or its library was switched off.
	removed, err := l.Store.PrunePlexItems(started)
	if err != nil {
		return "", err
	}
	l.lastFull.Store(time.Now().Unix())
	l.Store.SetSetting(keyLastFull, time.Now().UTC().Format(time.RFC3339))
	if len(removed) > 0 && l.OnRemoved != nil {
		l.OnRemoved(removed, false)
	}
	return fmt.Sprintf("%d titles in Plex (%d removed)", total, len(removed)), nil
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
	added, keys := 0, []string{}
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
			if id := m.tmdb(); id > 0 && m.DeletedAt == 0 {
				if isNew, _ := l.Store.SavePlexItem(store.PlexItem{Type: kindOf(sec.Type), TmdbID: id, RatingKey: m.RatingKey, Section: sec.Key, Title: m.Title, AddedAt: m.AddedAt, SeenAt: now}); isNew {
					added++
					keys = append(keys, fmt.Sprintf("%s:%d", kindOf(sec.Type), id))
				}
			}
		}
	}
	if len(keys) > 0 && l.OnAdded != nil {
		l.OnAdded(keys)
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
	web := "https://app.plex.tv/desktop"
	if own := l.Store.Setting(keyWebURL, ""); own != "" {
		web = own + "/web/index.html"
	}
	out := map[string]Item{}
	for k, it := range items {
		out[k] = Item{RatingKey: it.RatingKey, URL: fmt.Sprintf("%s#!/server/%s/details?key=%s", web, machine, url.QueryEscape("/library/metadata/"+it.RatingKey))}
	}
	return out, nil
}

// SetWebURL sets where Play on Plex and Open in Plex go: blank for app.plex.tv, or the address Plex's
// web app is reached on (like https://plex.example.com; a /web ending is fine).
func (l *Library) SetWebURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", l.Store.SetSetting(keyWebURL, "")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", errors.New("that address doesn't look right; it should be like https://plex.example.com")
	}
	p := strings.TrimRight(u.Path, "/")
	p = strings.TrimSuffix(strings.TrimSuffix(p, "/index.html"), "/web")
	u.Path, u.RawQuery, u.Fragment = p, "", ""
	clean := strings.TrimRight(u.String(), "/")
	return clean, l.Store.SetSetting(keyWebURL, clean)
}

// Seasons says how many episodes of each season of a series are in Plex (season 0 = specials).
// Empty when the series isn't in Plex.
func (l *Library) Seasons(ctx context.Context, tmdbID int) (map[int]int, error) {
	out := map[int]int{}
	items, err := l.Store.PlexItemsFor([]string{fmt.Sprintf("series:%d", tmdbID)})
	if err != nil || len(items) == 0 {
		return out, err
	}
	base, tok, ok := l.conn()
	if !ok {
		return out, nil
	}
	var res struct {
		MediaContainer struct {
			Metadata []struct {
				Index     int `json:"index"`
				LeafCount int `json:"leafCount"`
			} `json:"Metadata"`
		} `json:"MediaContainer"`
	}
	for _, it := range items {
		if err := l.get(ctx, base, "/library/metadata/"+url.PathEscape(it.RatingKey)+"/children", tok, &res); err != nil {
			return out, err
		}
	}
	for _, s := range res.MediaContainer.Metadata {
		out[s.Index] = s.LeafCount
	}
	return out, nil
}

// Episodes lists the episodes of a series that are in Plex, by season (specials left out). Empty when
// the series isn't in Plex.
func (l *Library) Episodes(ctx context.Context, tmdbID int) (map[int][]int, error) {
	out := map[int][]int{}
	items, err := l.Store.PlexItemsFor([]string{fmt.Sprintf("series:%d", tmdbID)})
	if err != nil || len(items) == 0 {
		return out, err
	}
	base, tok, ok := l.conn()
	if !ok {
		return out, nil
	}
	for _, it := range items {
		var res struct {
			MediaContainer struct {
				Metadata []struct {
					Season  int `json:"parentIndex"`
					Episode int `json:"index"`
				} `json:"Metadata"`
			} `json:"MediaContainer"`
		}
		if err := l.get(ctx, base, "/library/metadata/"+url.PathEscape(it.RatingKey)+"/allLeaves", tok, &res); err != nil {
			return out, err
		}
		for _, e := range res.MediaContainer.Metadata {
			if e.Season > 0 && !slices.Contains(out[e.Season], e.Episode) {
				out[e.Season] = append(out[e.Season], e.Episode)
			}
		}
	}
	return out, nil
}

// Connected reports whether a Plex server is set up.
func (l *Library) Connected() bool {
	_, _, ok := l.conn()
	return ok
}

// Rescan asks Plex to scan its chosen movie (or TV) libraries for new files, the same thing Radarr and
// Sonarr's own Plex connection does after an import.
func (l *Library) Rescan(ctx context.Context, typ string) error {
	base, tok, ok := l.conn()
	if !ok {
		return nil
	}
	secs, err := l.Sections(ctx)
	if err != nil {
		return err
	}
	want := map[string]string{"movie": "movie", "series": "show"}[typ]
	for _, sec := range secs {
		if sec.Enabled && sec.Type == want {
			var out any
			if err := l.get(ctx, base, "/library/sections/"+url.PathEscape(sec.Key)+"/refresh", tok, &out); err != nil {
				return err
			}
		}
	}
	return nil
}
