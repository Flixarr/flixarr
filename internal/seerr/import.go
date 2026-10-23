package seerr

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Flixarr/flixarr/internal/arr"
	"github.com/Flixarr/flixarr/internal/meta"
	"github.com/Flixarr/flixarr/internal/store"
)

// Seerr permission bits (the same in Overseerr and Jellyseerr).
const (
	permAdmin         = 2
	permManageReqs    = 16
	permRequest       = 32
	permAutoApprove   = 128
	permAutoMovie     = 256
	permAutoTv        = 512
	permAdvanced      = 8192
	permRequestMovie  = 262144
	permRequestTv     = 524288
	permManageIssues  = 1048576
	permCreateIssues  = 4194304
	permAutoRequest   = 8388608
	permAutoReqMovie  = 16777216
	permAutoReqTv     = 33554432
	requestPending    = 1
	requestDeclined   = 3
	issueResolved     = 2
	userTypePlex      = 1
	defaultQuotaDays  = 7
	titleLookupPause  = 120 * time.Millisecond // stays well under the API's 100 lookups per 10 seconds
	expiredReasonText = "Not in Plex, Radarr or Sonarr when it was brought over from Seerr."
)

// Perms turns Seerr's permission bits into Flixarr's switches.
func Perms(bits int64) []string {
	has := func(b int64) bool { return bits&b != 0 }
	if has(permAdmin) {
		return slices.Clone(store.AllPerms)
	}
	out := []string{}
	add := func(ok bool, p string) {
		if ok {
			out = append(out, p)
		}
	}
	add(has(permManageReqs), "manageRequests")
	add(has(permAutoApprove) || (has(permAutoMovie) && has(permAutoTv)), "autoApprove")
	add(has(permRequest) || has(permRequestMovie), "requestMovies")
	add(has(permRequest) || has(permRequestTv), "requestTv")
	add(has(permAdvanced), "advanced")
	add(has(permCreateIssues) || has(permManageIssues), "reportIssues")
	add(has(permManageIssues), "manageIssues")
	add(has(permAutoRequest) || has(permAutoReqMovie) || has(permAutoReqTv), "watchlist")
	return out
}

// weekly turns a Seerr limit (N per D days) into Flixarr's N per 7 days, rounding up.
func weekly(limit, days int) int {
	if limit <= 0 {
		return 0
	}
	if days <= 0 {
		days = defaultQuotaDays
	}
	n := (limit*7 + days - 1) / days
	return max(n, 1)
}

var issueKinds = map[int]string{1: "Video", 2: "Audio", 3: "Subtitles", 4: "Other"}

// ---------- the library: what Plex, Radarr and Sonarr have ----------

type arrMovie struct {
	ID      int
	HasFile bool
}

type arrSeries struct {
	ID      int
	Seasons map[int]int // season → episode files
}

// library is what's actually there, read once: statuses come from it, not from Seerr.
type library struct {
	plex     map[string]bool
	movies   map[int]arrMovie  // Radarr, by TMDB ID
	series   map[int]arrSeries // Sonarr, by TMDB ID
	byTvdb   map[int]arrSeries // Sonarr, by TVDB ID (older Sonarr has no TMDB IDs)
	radarrID int64             // the Flixarr server the requests belong to
	sonarrID int64
}

func readLibrary(ctx context.Context, st *store.Store, radarr, sonarr *store.Server) (*library, error) {
	l := &library{movies: map[int]arrMovie{}, series: map[int]arrSeries{}, byTvdb: map[int]arrSeries{}}
	var err error
	if l.plex, err = st.PlexKeys(); err != nil {
		return nil, err
	}
	if radarr != nil {
		l.radarrID = radarr.ID
		var list []struct {
			ID      int  `json:"id"`
			TmdbID  int  `json:"tmdbId"`
			HasFile bool `json:"hasFile"`
		}
		if err := (&arr.Client{Kind: "radarr", URL: radarr.URL, APIKey: radarr.APIKey}).Get(ctx, "movie", &list); err != nil {
			return nil, fmt.Errorf("Radarr: %w", err)
		}
		for _, m := range list {
			l.movies[m.TmdbID] = arrMovie{m.ID, m.HasFile}
		}
	}
	if sonarr != nil {
		l.sonarrID = sonarr.ID
		var list []struct {
			ID      int `json:"id"`
			TmdbID  int `json:"tmdbId"`
			TvdbID  int `json:"tvdbId"`
			Seasons []struct {
				SeasonNumber int `json:"seasonNumber"`
				Statistics   struct {
					EpisodeFileCount int `json:"episodeFileCount"`
				} `json:"statistics"`
			} `json:"seasons"`
		}
		if err := (&arr.Client{Kind: "sonarr", URL: sonarr.URL, APIKey: sonarr.APIKey}).Get(ctx, "series", &list); err != nil {
			return nil, fmt.Errorf("Sonarr: %w", err)
		}
		for _, s := range list {
			a := arrSeries{ID: s.ID, Seasons: map[int]int{}}
			for _, n := range s.Seasons {
				a.Seasons[n.SeasonNumber] = n.Statistics.EpisodeFileCount
			}
			if s.TmdbID > 0 {
				l.series[s.TmdbID] = a
			}
			if s.TvdbID > 0 {
				l.byTvdb[s.TvdbID] = a
			}
		}
	}
	return l, nil
}

// status works out where a Seerr request stands now: in Plex = available; files in Radarr/Sonarr but not
// in Plex yet = importing (Flixarr follows it from there); a request nobody decided yet stays waiting, a
// declined one stays declined; everything else never arrived or is gone since, so it's expired.
func (l *library) status(r Request) (status string, arrID int) {
	if r.Type == "movie" {
		m, inRadarr := l.movies[r.Media.TmdbID]
		if inRadarr {
			arrID = m.ID
		}
		switch {
		case l.plex[key(r)]:
			return "available", arrID
		case inRadarr && m.HasFile:
			return "importing", arrID
		}
	} else {
		s, inSonarr := l.series[r.Media.TmdbID]
		if !inSonarr && r.Media.TvdbID > 0 {
			s, inSonarr = l.byTvdb[r.Media.TvdbID]
		}
		if inSonarr {
			arrID = s.ID
		}
		if l.plex[key(r)] {
			return "available", arrID
		}
		if inSonarr {
			files := true
			for _, n := range seasonsOf(r) {
				files = files && s.Seasons[n] > 0
			}
			if files && len(seasonsOf(r)) > 0 {
				return "importing", arrID
			}
		}
	}
	switch r.Status {
	case requestPending:
		return "pending", arrID
	case requestDeclined:
		return "declined", arrID
	}
	return "expired", arrID
}

func kind(mediaType string) string {
	if mediaType == "tv" {
		return "series"
	}
	return "movie"
}

func key(r Request) string { return fmt.Sprintf("%s:%d", kind(r.Type), r.Media.TmdbID) }

func seasonsOf(r Request) []int {
	out := []int{}
	for _, s := range r.Seasons {
		if s.SeasonNumber > 0 && !slices.Contains(out, s.SeasonNumber) {
			out = append(out, s.SeasonNumber)
		}
	}
	sort.Ints(out)
	return out
}

// ---------- the plan: what an import would do ----------

// Preview is what importing would bring over, shown before anything is saved.
type Preview struct {
	Version  string       `json:"version"`
	Users    UsersPlan    `json:"users"`
	Requests RequestsPlan `json:"requests"`
	Issues   IssuesPlan   `json:"issues"`
	Servers  []ServerPlan `json:"servers"`
	Notes    []string     `json:"notes"`
	statuses map[int64]planned
}

type UsersPlan struct {
	Total      int `json:"total"`
	New        int `json:"new"`
	Existing   int `json:"existing"`   // already in Flixarr (same Plex account or local username)
	NoPassword int `json:"noPassword"` // local accounts that need a password set
	Imported   int `json:"imported"`   // brought over before
}

type RequestsPlan struct {
	Total    int            `json:"total"`
	Movies   int            `json:"movies"`
	Series   int            `json:"series"`
	Skipped  int            `json:"skipped"` // 4K
	Imported int            `json:"imported"`
	ByStatus map[string]int `json:"byStatus"`
}

type IssuesPlan struct {
	Total    int `json:"total"`
	Open     int `json:"open"`
	Resolved int `json:"resolved"`
	Imported int `json:"imported"`
}

type ServerPlan struct {
	Kind    string `json:"kind"`
	Name    string `json:"name"`
	Address string `json:"address"`
	Action  string `json:"action"` // add | kept | skipped
	Why     string `json:"why"`
	server  Server
}

type planned struct {
	status string
	arrID  int
	note   string // why an older duplicate ended
}

// Importer runs previews and imports, one import at a time.
type Importer struct {
	Store *store.Store
	Meta  *meta.Client
	Log   *slog.Logger
	// OnProgress is called as the import moves along (the server pushes it to the browser).
	OnProgress func(State)
	Pause      time.Duration // between title lookups; titleLookupPause when 0

	mu    sync.Mutex
	state State
}

// State is the running or last import.
type State struct {
	Running  bool    `json:"running"`
	Stage    string  `json:"stage"` // what it's doing now, in plain words
	Done     int     `json:"done"`
	Total    int     `json:"total"`
	Error    string  `json:"error,omitempty"`
	Result   *Result `json:"result,omitempty"`
	Finished string  `json:"finished,omitempty"`
}

// Result is what an import brought over.
type Result struct {
	Version  string   `json:"version"`
	Users    int      `json:"users"`
	Requests int      `json:"requests"`
	Issues   int      `json:"issues"`
	Servers  []string `json:"servers"`
	At       string   `json:"at"`
	// Plan is what the import worked out before bringing things over, for the summary (#126).
	Plan *Preview `json:"plan,omitempty"`
}

const keyLast = "seerr_import_last"

// State returns the running import, or the last one.
func (im *Importer) State() State {
	im.mu.Lock()
	defer im.mu.Unlock()
	s := im.state
	if !s.Running && s.Result == nil && s.Error == "" {
		var r Result
		if json.Unmarshal([]byte(im.Store.Setting(keyLast, "")), &r) == nil && r.At != "" {
			s.Result, s.Finished = &r, r.At
		}
	}
	return s
}

func (im *Importer) set(f func(*State)) {
	im.mu.Lock()
	f(&im.state)
	s := im.state
	im.mu.Unlock()
	if im.OnProgress != nil {
		im.OnProgress(s)
	}
}

// serversFor picks, per kind, the Flixarr server requests go to and the Seerr server that would be added
// when Flixarr has none (Seerr's default non-4K one).
func serversFor(st *store.Store, d *Data) (radarr, sonarr *store.Server, plans []ServerPlan, err error) {
	have, err := st.Servers()
	if err != nil {
		return nil, nil, nil, err
	}
	for _, kind := range []string{"radarr", "sonarr"} {
		var mine *store.Server
		for i := range have {
			if have[i].Kind == kind {
				mine = &have[i]
				break
			}
		}
		list := d.Radarr
		if kind == "sonarr" {
			list = d.Sonarr
		}
		pick := -1
		for i, s := range list {
			if !s.Is4k && (pick < 0 || s.IsDefault) {
				pick = i
			}
		}
		for i, s := range list {
			p := ServerPlan{Kind: kind, Name: s.Name, Address: s.Address(), server: s}
			switch {
			case s.Is4k:
				p.Action, p.Why = "skipped", "4K servers aren't used yet (one copy per title)."
			case i != pick:
				p.Action, p.Why = "skipped", "Flixarr uses one "+label(kind)+" for now."
			case mine != nil:
				p.Action, p.Why = "kept", "Already connected, kept as it is."
			default:
				p.Action, p.Why = "add", "Will be added."
				sv := toServer(kind, s)
				if kind == "radarr" {
					radarr = &sv
				} else {
					sonarr = &sv
				}
			}
			plans = append(plans, p)
		}
		if mine != nil {
			if kind == "radarr" {
				radarr = mine
			} else {
				sonarr = mine
			}
		}
	}
	return radarr, sonarr, plans, nil
}

func label(kind string) string {
	if kind == "sonarr" {
		return "Sonarr"
	}
	return "Radarr"
}

func toServer(kind string, s Server) store.Server {
	name := s.Name
	if name == "" {
		name = label(kind)
	}
	web := s.ExternalURL
	return store.Server{Kind: kind, Name: name, URL: s.Address(), WebURL: web, APIKey: s.APIKey, RootFolder: s.ActiveDirectory,
		QualityProfileID: s.ActiveProfileID, QualityProfileName: s.ActiveProfileName, SeasonFolders: kind == "sonarr" && s.EnableSeasonFolders,
		SearchOnAdd: !s.PreventSearch, Enabled: true}
}

// Plan reads Seerr and works out what importing would do. Nothing is saved.
func (im *Importer) Plan(ctx context.Context, src Source) (*Preview, *Data, *library, error) {
	d, err := src.Fetch(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	radarr, sonarr, servers, err := serversFor(im.Store, d)
	if err != nil {
		return nil, nil, nil, err
	}
	lib, err := readLibrary(ctx, im.Store, radarr, sonarr)
	if err != nil {
		return nil, nil, nil, err
	}
	p := &Preview{Version: d.Version, Servers: servers, Notes: []string{}, statuses: map[int64]planned{}}
	if p.Servers == nil {
		p.Servers = []ServerPlan{}
	}

	// Users
	seenUsers, _ := im.Store.SeerrIDs("users")
	for _, u := range d.Users {
		p.Users.Total++
		switch {
		case seenUsers[u.ID] != 0:
			p.Users.Imported++
		case im.existingUser(u) != nil:
			p.Users.Existing++
		default:
			p.Users.New++
			if u.UserType != userTypePlex || u.PlexID == 0 {
				p.Users.NoPassword++
			}
		}
	}

	// Requests: oldest first, so for a movie asked for more than once only the newest stays in play.
	seenReqs, _ := im.Store.SeerrIDs("requests")
	p.Requests.ByStatus = map[string]int{}
	reqs := slices.Clone(d.Requests)
	sort.SliceStable(reqs, func(a, b int) bool { return reqs[a].CreatedAt < reqs[b].CreatedAt })
	openMovie := map[int]int64{} // TMDB ID → the Seerr request in play
	for _, r := range reqs {
		p.Requests.Total++
		if r.Is4k {
			p.Requests.Skipped++
			continue
		}
		if r.Type == "movie" {
			p.Requests.Movies++
		} else {
			p.Requests.Series++
		}
		if seenReqs[r.ID] != 0 {
			p.Requests.Imported++
			continue
		}
		s, arrID := lib.status(r)
		pl := planned{status: s, arrID: arrID}
		if r.Type == "movie" && slices.Contains(store.OpenStatuses, s) {
			if prev, ok := openMovie[r.Media.TmdbID]; ok {
				old := p.statuses[prev]
				p.Requests.ByStatus[old.status]--
				old.status, old.note = "expired", "Requested again later."
				p.statuses[prev] = old
				p.Requests.ByStatus["expired"]++
			} else if _, open, _ := im.Store.OpenRequest("movie", r.Media.TmdbID); open {
				pl.status, pl.note = "expired", "Already requested in Flixarr."
			}
			if pl.status != "expired" {
				openMovie[r.Media.TmdbID] = r.ID
			}
		}
		p.statuses[r.ID] = pl
		p.Requests.ByStatus[pl.status]++
	}

	// Issues
	seenIssues, _ := im.Store.SeerrIDs("issues")
	for _, i := range d.Issues {
		p.Issues.Total++
		if seenIssues[i.ID] != 0 {
			p.Issues.Imported++
		} else if i.Status == issueResolved {
			p.Issues.Resolved++
		} else {
			p.Issues.Open++
		}
	}

	if p.Users.NoPassword > 0 {
		if p.Users.NoPassword == 1 {
			p.Notes = append(p.Notes, "1 account without Plex comes over with no password. Set one for it in Users.")
		} else {
			p.Notes = append(p.Notes, plural(p.Users.NoPassword, "account")+" without Plex come over with no password. Set one for each in Users.")
		}
	}
	if p.Users.Existing > 0 {
		p.Notes = append(p.Notes, "People already in Flixarr keep their permissions.")
	}
	if p.Requests.Skipped > 0 {
		p.Notes = append(p.Notes, plural(p.Requests.Skipped, "4K request")+" left out (one copy per title for now).")
	}
	p.Notes = append(p.Notes, "Notification settings don't come over. Set them up in Notifications.")
	return p, d, lib, nil
}

func plural(n int, what string) string {
	if n == 1 {
		return "1 " + what
	}
	return fmt.Sprintf("%d %ss", n, what)
}

// existingUser finds the Flixarr person a Seerr user already is: same Plex account, or for accounts
// without Plex, a local account with the same username.
func (im *Importer) existingUser(u User) *store.User {
	if u.PlexID != 0 {
		if f, err := im.Store.UserByPlexID(u.PlexID); err == nil {
			return f
		}
		return nil
	}
	if f, err := im.Store.LocalUserByName(u.Name()); err == nil {
		return f
	}
	return nil
}

// Start reads Seerr again and imports in the background; done (if set) runs when it ends. It refuses
// while an import is running.
func (im *Importer) Start(src Source, done func()) error {
	im.mu.Lock()
	if im.state.Running {
		im.mu.Unlock()
		return fmt.Errorf("an import is already running")
	}
	im.state = State{Running: true, Stage: "Reading Seerr"}
	im.mu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Hour)
		defer cancel()
		// Title lookups go at the background pace (#165): an import is thousands of titles no cache has,
		// and many installs importing at once must not swamp the API.
		res, err := im.Run(meta.Background(ctx), src)
		if done != nil {
			done()
		}
		im.set(func(s *State) {
			s.Running, s.Finished = false, time.Now().UTC().Format(time.RFC3339)
			if err != nil {
				s.Error, s.Stage = capitalize(err.Error())+".", "Stopped"
			} else {
				s.Result, s.Stage = res, "Done"
			}
		})
	}()
	return nil
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// Run imports everything (the caller shows progress through OnProgress).
func (im *Importer) Run(ctx context.Context, src Source) (*Result, error) {
	p, d, lib, err := im.Plan(ctx, src)
	if err != nil {
		return nil, err
	}
	res := &Result{Version: d.Version, Servers: []string{}, Plan: p}
	now := time.Now().UTC().Format(time.RFC3339)

	// Radarr / Sonarr first, so requests belong to them.
	for _, sp := range p.Servers {
		if sp.Action != "add" {
			continue
		}
		sv, err := im.Store.SaveServer(toServer(sp.Kind, sp.server))
		if err != nil {
			return nil, fmt.Errorf("couldn't add %s: %w", label(sp.Kind), err)
		}
		if sp.Kind == "radarr" {
			lib.radarrID = sv.ID
		} else {
			lib.sonarrID = sv.ID
		}
		res.Servers = append(res.Servers, sv.Name)
	}

	// People
	im.set(func(s *State) { s.Stage, s.Done, s.Total = "Bringing over people", 0, len(d.Users) })
	users, err := im.Store.SeerrIDs("users")
	if err != nil {
		return nil, err
	}
	for i, u := range d.Users {
		if users[u.ID] == 0 {
			if f := im.existingUser(u); f != nil {
				im.Store.LinkSeerrUser(f.ID, u.ID)
				users[u.ID] = f.ID
			} else {
				nu := store.User{PlexID: u.PlexID, Username: u.Name(), Email: u.Email, Perms: Perms(u.Permissions), CreatedAt: u.CreatedAt,
					MovieLimit: weekly(u.MovieQuotaLimit, u.MovieQuotaDays), SeasonLimit: weekly(u.TvQuotaLimit, u.TvQuotaDays)}
				if u.UserType != userTypePlex {
					nu.PlexID = 0
				}
				if strings.HasPrefix(u.Avatar, "https://") {
					nu.Avatar = u.Avatar
				}
				if slices.Contains(nu.Perms, "admin") {
					nu.MovieLimit, nu.SeasonLimit = 0, 0
				}
				nu.Role = store.RoleOf(nu.Perms)
				f, err := im.Store.ImportUser(nu, u.ID)
				if err != nil {
					return nil, fmt.Errorf("couldn't add %s: %w", u.Name(), err)
				}
				users[u.ID] = f.ID
				res.Users++
			}
		}
		im.set(func(s *State) { s.Done = i + 1 })
	}

	// Titles: looked up once each for their name, year, poster and seasons.
	titles := map[string]title{}
	look := func(typ string, id int) title {
		k := fmt.Sprintf("%s:%d", typ, id)
		if t, ok := titles[k]; ok {
			return t
		}
		t := im.lookup(ctx, typ, id)
		titles[k] = t
		return t
	}

	// Requests
	reqs := slices.Clone(d.Requests)
	sort.SliceStable(reqs, func(a, b int) bool { return reqs[a].CreatedAt < reqs[b].CreatedAt })
	seen, err := im.Store.SeerrIDs("requests")
	if err != nil {
		return nil, err
	}
	im.set(func(s *State) { s.Stage, s.Done, s.Total = "Bringing over requests", 0, len(reqs) })
	for i, r := range reqs {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		pl, ok := p.statuses[r.ID]
		uid := users[r.RequestedBy.ID]
		if ok && seen[r.ID] == 0 && !r.Is4k && uid != 0 && r.Media.TmdbID > 0 {
			t := look(kind(r.Type), r.Media.TmdbID)
			fr, events, decider := im.request(r, pl, t, lib, users, now)
			fr.UserID = uid
			if _, err := im.Store.ImportRequest(fr, r.ID, decider, events); err != nil {
				return nil, fmt.Errorf("couldn't save a request for %s: %w", t.Title, err)
			}
			res.Requests++
		}
		im.set(func(s *State) { s.Done = i + 1 })
	}

	// Issues
	seenIssues, err := im.Store.SeerrIDs("issues")
	if err != nil {
		return nil, err
	}
	im.set(func(s *State) { s.Stage, s.Done, s.Total = "Bringing over issues", 0, len(d.Issues) })
	for n, is := range d.Issues {
		uid := users[is.CreatedBy.ID]
		if seenIssues[is.ID] == 0 && uid != 0 && is.Media.TmdbID > 0 {
			typ := kind(is.Media.MediaType)
			t := look(typ, is.Media.TmdbID)
			fi := store.Issue{Type: typ, TmdbID: is.Media.TmdbID, Title: t.Title, Year: t.Year, Poster: t.Poster, Kind: issueKinds[is.IssueType],
				UserID: uid, Status: "open", CreatedAt: is.CreatedAt, UpdatedAt: is.UpdatedAt}
			if fi.Kind == "" {
				fi.Kind = "Other"
			}
			if is.Status == issueResolved {
				fi.Status = "resolved"
			}
			if typ == "series" && is.ProblemSeason > 0 {
				fi.Season, fi.Episode = is.ProblemSeason, is.ProblemEpisode
				fi.Parts = [][2]int{{is.ProblemSeason, is.ProblemEpisode}}
			}
			comments := []store.Comment{}
			for k, c := range is.Comments {
				who := uid
				if c.User != nil && users[c.User.ID] != 0 {
					who = users[c.User.ID]
				}
				if k == 0 && who == uid && fi.Note == "" { // Seerr keeps the reporter's description as the first comment
					fi.Note = c.Message
					continue
				}
				comments = append(comments, store.Comment{UserID: who, Body: c.Message, CreatedAt: c.CreatedAt})
			}
			var resolver int64
			if is.ModifiedBy != nil {
				resolver = users[is.ModifiedBy.ID]
			}
			if _, err := im.Store.ImportIssue(fi, is.ID, resolver, comments); err != nil {
				return nil, fmt.Errorf("couldn't save an issue for %s: %w", t.Title, err)
			}
			res.Issues++
		}
		im.set(func(s *State) { s.Done = n + 1 })
	}

	res.At = time.Now().UTC().Format(time.RFC3339)
	b, _ := json.Marshal(res)
	im.Store.SetSetting(keyLast, string(b))
	if im.Log != nil {
		im.Log.Info("imported from Seerr", "version", res.Version, "users", res.Users, "requests", res.Requests, "issues", res.Issues)
	}
	return res, nil
}

// request builds the Flixarr request and its timeline for a Seerr request.
func (im *Importer) request(r Request, pl planned, t title, lib *library, users map[int64]int64, now string) (store.Request, []store.Event, int64) {
	typ := kind(r.Type)
	fr := store.Request{Type: typ, TmdbID: r.Media.TmdbID, Title: t.Title, Year: t.Year, Poster: t.Poster, Backdrop: t.Backdrop,
		Status: pl.status, ArrID: pl.arrID, ProfileID: r.ProfileID, ProfileName: r.ProfileName, Mode: "all", Seasons: []int{},
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Source: "seerr"}
	if typ == "movie" {
		fr.ServerID = lib.radarrID
	} else {
		fr.ServerID = lib.sonarrID
		fr.Anime = t.Anime
		want := seasonsOf(r)
		if len(want) > 0 && !covers(want, t.Seasons) {
			fr.Mode, fr.Seasons = "seasons", want
		}
		fr.SeasonCount = len(want)
	}
	if fr.UpdatedAt == "" {
		fr.UpdatedAt = fr.CreatedAt
	}
	events := []store.Event{{Status: "requested", At: r.CreatedAt}}
	var decider int64
	if r.ModifiedBy != nil {
		decider = users[r.ModifiedBy.ID]
	}
	who := ""
	if r.ModifiedBy != nil {
		who = r.ModifiedBy.Name()
	}
	if r.Status == requestDeclined {
		events = append(events, store.Event{Status: "declined", Note: who, At: r.UpdatedAt})
	} else if r.Status != requestPending {
		events = append(events, store.Event{Status: "approved", Note: who, At: r.UpdatedAt})
	}
	switch pl.status {
	case "available":
		at := r.Media.MediaAddedAt
		if at == "" || at < r.CreatedAt {
			at = fr.UpdatedAt
		}
		events = append(events, store.Event{Status: "available", At: at})
		fr.UpdatedAt = max(fr.UpdatedAt, at)
	case "importing":
		events = append(events, store.Event{Status: "importing", At: now})
	case "expired":
		note := pl.note
		if note == "" {
			note = expiredReasonText
		}
		fr.Error = note
		events = append(events, store.Event{Status: "expired", Note: note, At: now})
	}
	return fr, events, decider
}

// covers: the seasons asked for are every season the show has (so it's a whole-show request).
func covers(want, all []int) bool {
	if len(all) == 0 {
		return false
	}
	for _, n := range all {
		if !slices.Contains(want, n) {
			return false
		}
	}
	return true
}

// title is what a request or issue shows about its title.
type title struct {
	Title, Year, Poster, Backdrop string
	Seasons                       []int
	Anime                         bool
}

// lookup reads a title from the Flixarr API (through the app cache), pausing between lookups that
// weren't cached so a big import stays under the API's limit. A title the API doesn't know keeps its ID.
func (im *Importer) lookup(ctx context.Context, typ string, id int) title {
	t := title{Title: fmt.Sprintf("TMDB %d", id)}
	if im.Meta == nil {
		return t
	}
	before := im.Meta.Counters().Misses
	body, status, err := im.Meta.Get(ctx, fmt.Sprintf("%s/%d", typ, id), url.Values{})
	if im.Meta.Counters().Misses != before {
		pause := im.Pause
		if pause == 0 {
			pause = titleLookupPause
		}
		select {
		case <-time.After(pause):
		case <-ctx.Done():
		}
	}
	if err != nil || status != http.StatusOK {
		return t
	}
	var m struct {
		Title    string   `json:"title"`
		Year     string   `json:"year"`
		Poster   string   `json:"poster"`
		Backdrop string   `json:"backdrop"`
		Language string   `json:"language"`
		Genres   []string `json:"genres"`
		Seasons  []struct {
			N int `json:"n"`
		} `json:"seasons"`
	}
	if json.Unmarshal(body, &m) != nil || m.Title == "" {
		return t
	}
	t = title{Title: m.Title, Year: m.Year, Poster: m.Poster, Backdrop: m.Backdrop}
	for _, s := range m.Seasons {
		if s.N > 0 {
			t.Seasons = append(t.Seasons, s.N)
		}
	}
	if typ == "series" && m.Language == "ja" {
		for _, g := range m.Genres {
			t.Anime = t.Anime || strings.EqualFold(g, "Animation")
		}
	}
	return t
}
