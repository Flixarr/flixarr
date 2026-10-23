package seerr

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/Flixarr/flixarr/internal/secret"
	"github.com/Flixarr/flixarr/internal/store"
)

func TestPerms(t *testing.T) {
	// A typical Seerr user: request, request view, advanced, auto approve, issues, recent view.
	got := Perms(74211488)
	for _, p := range []string{"requestMovies", "requestTv", "advanced", "autoApprove", "reportIssues"} {
		if !slices.Contains(got, p) {
			t.Errorf("missing %s in %v", p, got)
		}
	}
	if slices.Contains(got, "admin") || slices.Contains(got, "manageRequests") {
		t.Errorf("too much: %v", got)
	}
	if !slices.Equal(Perms(2), store.AllPerms) {
		t.Error("admin bit isn't every switch")
	}
	if p := Perms(permRequestMovie | permAutoMovie); slices.Contains(p, "requestTv") || slices.Contains(p, "autoApprove") {
		t.Errorf("movie only: %v", p)
	}
}

func TestWeekly(t *testing.T) {
	for _, c := range [][3]int{{0, 7, 0}, {5, 7, 5}, {10, 30, 3}, {1, 30, 1}, {3, 1, 21}, {4, 0, 4}} {
		if got := weekly(c[0], c[1]); got != c[2] {
			t.Errorf("weekly(%d, %d) = %d, want %d", c[0], c[1], got, c[2])
		}
	}
}

func TestServerAddress(t *testing.T) {
	if a := (Server{Hostname: "radarr", Port: 7878}).Address(); a != "http://radarr:7878" {
		t.Error(a)
	}
	if a := (Server{Hostname: "x.example.com", Port: 443, UseSsl: true, BaseURL: "radarr/"}).Address(); a != "https://x.example.com:443/radarr" {
		t.Error(a)
	}
}

func TestNewClient(t *testing.T) {
	if _, err := NewClient("192.168.1.10:5055", ""); err == nil {
		t.Error("no key accepted")
	}
	if _, err := NewClient("ftp://x", "k"); err == nil {
		t.Error("ftp accepted")
	}
	c, err := NewClient(" 192.168.1.10:5055/api/v1/ ", "k")
	if err != nil || c.URL != "http://192.168.1.10:5055" {
		t.Fatalf("%v %v", c, err)
	}
}

// fakeSeerr answers like Seerr: paged lists, issue details, the Radarr/Sonarr settings.
func fakeSeerr(t *testing.T, arrURL string) *httptest.Server {
	users := []map[string]any{
		{"id": 1, "userType": 1, "plexId": 100, "plexUsername": "owner", "email": "o@x.test", "permissions": 2, "createdAt": "2022-01-01T00:00:00.000Z"},
		{"id": 2, "userType": 1, "plexId": 200, "plexUsername": "eryn", "email": "e@x.test", "avatar": "https://plex.tv/users/abc/avatar?c=1", "permissions": 74211488, "movieQuotaLimit": 10, "movieQuotaDays": 30, "createdAt": "2023-01-01T00:00:00.000Z"},
		{"id": 3, "userType": 2, "username": "grandma", "displayName": "Grandma", "permissions": 32, "createdAt": "2024-01-01T00:00:00.000Z"},
	}
	req := func(id int, typ string, tmdb, tvdb, status, user int, created string, seasons ...int) map[string]any {
		ss := []map[string]any{}
		for _, n := range seasons {
			ss = append(ss, map[string]any{"seasonNumber": n})
		}
		return map[string]any{"id": id, "type": typ, "status": status, "is4k": false, "createdAt": created, "updatedAt": created,
			"media":   map[string]any{"tmdbId": tmdb, "tvdbId": tvdb, "mediaType": typ, "status": 5, "mediaAddedAt": created},
			"seasons": ss, "requestedBy": users[user-1], "modifiedBy": users[0]}
	}
	requests := []map[string]any{
		req(1, "movie", 603, 0, 5, 2, "2023-02-01T00:00:00.000Z"),      // in Plex → available
		req(2, "movie", 604, 0, 2, 2, "2023-02-02T00:00:00.000Z"),      // file in Radarr → importing
		req(3, "movie", 605, 0, 2, 3, "2023-02-03T00:00:00.000Z"),      // nowhere → expired (the stale "approved" case)
		req(4, "movie", 605, 0, 2, 2, "2024-02-03T00:00:00.000Z"),      // asked again: the newer one still expired (nowhere)
		req(5, "movie", 603, 0, 5, 3, "2024-03-01T00:00:00.000Z"),      // in Plex, asked again: newer stays, older expires
		req(6, "tv", 1396, 81189, 2, 2, "2023-03-01T00:00:00.000Z", 1), // season 1 has files in Sonarr → importing
		req(7, "tv", 1399, 121361, 3, 3, "2023-03-02T00:00:00.000Z", 1, 2),
		req(8, "movie", 606, 0, 1, 3, "2023-04-01T00:00:00.000Z"), // still waiting
	}
	r4k := req(9, "movie", 607, 0, 2, 2, "2023-05-01T00:00:00.000Z")
	r4k["is4k"] = true
	requests = append(requests, r4k)
	issues := []map[string]any{
		{"id": 1, "issueType": 2, "status": 1, "problemSeason": 0, "problemEpisode": 0, "createdAt": "2023-06-01T00:00:00.000Z", "updatedAt": "2023-06-01T00:00:00.000Z",
			"media": map[string]any{"tmdbId": 603, "mediaType": "movie"}, "createdBy": users[1],
			"comments": []map[string]any{{"message": "Way too quiet", "user": users[1], "createdAt": "2023-06-01T00:00:00.000Z"}, {"message": "On it", "user": users[0], "createdAt": "2023-06-02T00:00:00.000Z"}}},
		{"id": 2, "issueType": 3, "status": 2, "problemSeason": 2, "problemEpisode": 4, "createdAt": "2023-07-01T00:00:00.000Z", "updatedAt": "2023-07-02T00:00:00.000Z",
			"media": map[string]any{"tmdbId": 1396, "mediaType": "tv"}, "createdBy": users[2], "modifiedBy": users[0], "comments": []map[string]any{}},
	}
	paged := func(w http.ResponseWriter, r *http.Request, all []map[string]any) {
		take, _ := strconv.Atoi(r.URL.Query().Get("take"))
		skip, _ := strconv.Atoi(r.URL.Query().Get("skip"))
		end := min(skip+take, len(all))
		json.NewEncoder(w).Encode(map[string]any{"pageInfo": map[string]any{"results": len(all), "pages": 1}, "results": all[min(skip, len(all)):end]})
	}
	host, port, _ := strings.Cut(strings.TrimPrefix(arrURL, "http://"), ":")
	p, _ := strconv.Atoi(port)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch path := strings.TrimPrefix(r.URL.Path, "/api/v1/"); {
		case path == "status":
			w.Write([]byte(`{"version":"3.5.0"}`))
		case path == "user":
			paged(w, r, users)
		case path == "request":
			paged(w, r, requests)
		case path == "issue":
			paged(w, r, issues)
		case strings.HasPrefix(path, "issue/"):
			n, _ := strconv.Atoi(strings.TrimPrefix(path, "issue/"))
			json.NewEncoder(w).Encode(issues[n-1])
		case path == "settings/radarr":
			json.NewEncoder(w).Encode([]map[string]any{{"name": "Radarr", "hostname": host, "port": p, "apiKey": "rk", "activeDirectory": "/movies", "activeProfileId": 1, "isDefault": true},
				{"name": "Radarr 4K", "hostname": host, "port": p, "apiKey": "rk", "is4k": true}})
		case path == "settings/sonarr":
			json.NewEncoder(w).Encode([]map[string]any{{"name": "Sonarr", "hostname": host, "port": p, "apiKey": "sk", "activeDirectory": "/tv", "activeProfileId": 7, "isDefault": true, "enableSeasonFolders": true}})
		default:
			http.NotFound(w, r)
		}
	}))
}

// fakeArr is one server answering as Radarr (/api/v3/movie) and Sonarr (/api/v3/series).
func fakeArr() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v3/movie":
			w.Write([]byte(`[{"id":11,"tmdbId":603,"hasFile":true},{"id":12,"tmdbId":604,"hasFile":true},{"id":13,"tmdbId":605,"hasFile":false}]`))
		case "/api/v3/series":
			w.Write([]byte(`[{"id":21,"tvdbId":81189,"seasons":[{"seasonNumber":1,"statistics":{"episodeFileCount":7}},{"seasonNumber":2,"statistics":{"episodeFileCount":0}}]}]`))
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestPreviewAndImport(t *testing.T) {
	arrSrv := fakeArr()
	defer arrSrv.Close()
	srv := fakeSeerr(t, arrSrv.URL)
	defer srv.Close()
	dir := t.TempDir()
	box, _ := secret.Load(dir)
	st, err := store.Open(dir, box)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	owner, _ := st.CreateUser(store.User{PlexID: 100, Username: "owner", Role: "Admin"})
	st.SavePlexItem(store.PlexItem{Type: "movie", TmdbID: 603, RatingKey: "1", SeenAt: "2026-01-01T00:00:00Z"})

	c, _ := NewClient(srv.URL, "key")
	im := &Importer{Store: st}
	p, _, _, err := im.Plan(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	if p.Version != "3.5.0" || p.Users.Total != 3 || p.Users.New != 2 || p.Users.Existing != 1 || p.Users.NoPassword != 1 {
		t.Errorf("users %+v", p.Users)
	}
	if p.Requests.Total != 9 || p.Requests.Skipped != 1 || p.Requests.Movies != 6 || p.Requests.Series != 2 {
		t.Errorf("requests %+v", p.Requests)
	}
	want := map[string]int{"available": 1, "importing": 2, "expired": 3, "declined": 1, "pending": 1}
	for k, n := range want {
		if p.Requests.ByStatus[k] != n {
			t.Errorf("%s: %d, want %d (%v)", k, p.Requests.ByStatus[k], n, p.Requests.ByStatus)
		}
	}
	if p.Issues.Open != 1 || p.Issues.Resolved != 1 {
		t.Errorf("issues %+v", p.Issues)
	}
	actions := []string{}
	for _, s := range p.Servers {
		actions = append(actions, s.Kind+":"+s.Action)
	}
	if fmt.Sprint(actions) != "[radarr:add radarr:skipped sonarr:add]" {
		t.Errorf("servers %v", actions)
	}
	if n, _ := st.UserCount(); n != 1 {
		t.Fatal("preview saved something")
	}

	res, err := im.Run(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	if res.Users != 2 || res.Requests != 8 || res.Issues != 2 || len(res.Servers) != 2 {
		t.Fatalf("result %+v", res)
	}
	// The owner keeps their own permissions; eryn's 10 per 30 days becomes 3 per week; grandma has no password.
	if o, _ := st.UserByPlexID(100); !o.Owner || o.ID != owner.ID {
		t.Error("owner changed")
	}
	eryn, _ := st.UserByPlexID(200)
	if eryn == nil || eryn.MovieLimit != 3 || eryn.CreatedAt != "2023-01-01T00:00:00.000Z" || !eryn.Can("autoApprove") || eryn.Avatar == "" {
		t.Fatalf("eryn %+v", eryn)
	}
	gm, err := st.LocalUserByName("grandma")
	if err != nil || st.PasswordHash(gm.ID) != "" {
		t.Fatalf("grandma %+v %v", gm, err)
	}
	list, _ := st.RequestsForTitle("movie", 603)
	statuses := []string{}
	for _, r := range list {
		statuses = append(statuses, r.Status+"/"+r.Source)
	}
	slices.Sort(statuses)
	if fmt.Sprint(statuses) != "[available/seerr expired/seerr]" {
		t.Errorf("603: %v", statuses)
	}
	tv, _ := st.RequestsForTitle("series", 1396)
	if len(tv) != 1 || tv[0].Status != "importing" || tv[0].ArrID != 21 || tv[0].ServerID == 0 || tv[0].CreatedAt != "2023-03-01T00:00:00.000Z" {
		t.Errorf("1396: %+v", tv)
	}
	ev, _ := st.RequestEvents(tv[0].ID)
	if len(ev) < 2 || ev[0].Status != "requested" || ev[0].At != "2023-03-01T00:00:00.000Z" {
		t.Errorf("timeline %+v", ev)
	}
	issues, _ := st.Issues(0, "")
	if len(issues) != 2 {
		t.Fatalf("issues %d", len(issues))
	}
	for _, i := range issues {
		if i.TmdbID == 603 {
			cm, _ := st.Comments(i.ID)
			if i.Note != "Way too quiet" || i.Kind != "Audio" || len(cm) != 1 || cm[0].UserID != owner.ID {
				t.Errorf("issue %+v comments %+v", i, cm)
			}
		} else if i.Status != "resolved" || i.Season != 2 || i.Episode != 4 {
			t.Errorf("issue %+v", i)
		}
	}
	servers, _ := st.Servers()
	if len(servers) != 2 || servers[0].APIKey == "" || servers[0].RootFolder == "" {
		t.Fatalf("servers %+v", servers)
	}

	// Importing again adds nothing and changes nothing.
	again, err := im.Run(context.Background(), c)
	if err != nil || again.Users != 0 || again.Requests != 0 || again.Issues != 0 || len(again.Servers) != 0 {
		t.Fatalf("again %+v %v", again, err)
	}
	if n, _ := st.UserCount(); n != 3 {
		t.Fatalf("%d users", n)
	}
	if s := im.State(); s.Result == nil || s.Result.Version != "3.5.0" {
		t.Fatalf("last import not remembered: %+v", s)
	}
}

func TestWrongKey(t *testing.T) {
	srv := fakeSeerr(t, "http://127.0.0.1:1")
	defer srv.Close()
	c, _ := NewClient(srv.URL, "nope")
	if _, err := c.Fetch(context.Background()); err == nil || !strings.Contains(err.Error(), "API key") {
		t.Fatalf("%v", err)
	}
}
