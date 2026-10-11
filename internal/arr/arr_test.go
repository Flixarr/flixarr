package arr

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func fake(app string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "good" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/api/v3/system/status":
			w.Write([]byte(`{"appName":"` + app + `","instanceName":"` + app + `","version":"6.4.4.10685"}`))
		case "/api/v3/qualityprofile":
			w.Write([]byte(`[{"id":1,"name":"Default Quality","items":[]},{"id":26,"name":"Prefer 4K"}]`))
		case "/api/v3/rootfolder":
			w.Write([]byte(`[{"id":5,"path":"/mnt/media/movies","freeSpace":7202611499008,"accessible":true}]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestTest(t *testing.T) {
	srv := fake("Radarr")
	defer srv.Close()
	info, err := (&Client{Kind: "radarr", URL: srv.URL, APIKey: "good"}).Test(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Version != "6.4.4.10685" || len(info.Profiles) != 2 || info.Profiles[1].Name != "Prefer 4K" || info.Folders[0].Path != "/mnt/media/movies" {
		t.Errorf("unexpected info: %+v", info)
	}
	if _, err := (&Client{Kind: "radarr", URL: srv.URL, APIKey: "bad"}).Test(context.Background()); err == nil || !strings.Contains(err.Error(), "API key") {
		t.Errorf("bad key error = %v", err)
	}
	if _, err := (&Client{Kind: "sonarr", URL: srv.URL, APIKey: "good"}).Test(context.Background()); err == nil || !strings.Contains(err.Error(), "that's Radarr, not Sonarr") {
		t.Errorf("wrong app error = %v", err)
	}
	if _, err := (&Client{Kind: "radarr", URL: "http://127.0.0.1:1", APIKey: "good"}).Test(context.Background()); err == nil || !strings.Contains(err.Error(), "reach") {
		t.Errorf("unreachable error = %v", err)
	}
}

func TestNormalizeURL(t *testing.T) {
	for in, want := range map[string]string{
		"10.0.0.5:7878":                       "http://10.0.0.5:7878",
		"http://10.0.0.5:7878/":               "http://10.0.0.5:7878",
		"https://example.com/radarr/":         "https://example.com/radarr",
		"https://example.com/radarr/api/v3":   "https://example.com/radarr",
		" http://sonarr:8989/api ":            "http://sonarr:8989",
		"http://10.0.0.5:7878/?apikey=x#frag": "http://10.0.0.5:7878",
	} {
		if got, err := NormalizeURL(in); err != nil || got != want {
			t.Errorf("NormalizeURL(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "ftp://x", "http://"} {
		if _, err := NormalizeURL(bad); err == nil {
			t.Errorf("NormalizeURL(%q) should fail", bad)
		}
	}
}

func TestProbe(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ping":
			w.Write([]byte(`{"status":"OK"}`))
		case "/":
			http.Redirect(w, r, "/login", http.StatusFound)
		case "/login":
			w.Write([]byte(`<html><head><title>Login - Radarr</title></head></html>`))
		}
	}))
	defer srv.Close()
	hc := srv.Client()
	if !probe(context.Background(), hc, srv.URL, "Radarr") {
		t.Error("Radarr not recognized")
	}
	if probe(context.Background(), hc, srv.URL, "Sonarr") {
		t.Error("Radarr taken for Sonarr")
	}
	if probe(context.Background(), hc, "http://127.0.0.1:1", "Radarr") {
		t.Error("closed port recognized")
	}
}

func TestRowAndHelpers(t *testing.T) {
	var r queueRecord
	r.Title, r.Size, r.SizeLeft, r.TimeLeft, r.Status = "Rugrats.S01E07.1080p", 1000, 400, "00:04:12", "downloading"
	r.Series = &struct {
		Title string `json:"title"`
	}{"Rugrats"}
	r.Episode = &struct {
		SeasonNumber  int `json:"seasonNumber"`
		EpisodeNumber int `json:"episodeNumber"`
	}{1, 7}
	if q := row(r); q.Title != "Rugrats S01E07" || q.Percent != 60 || q.State != "downloading" || q.TimeLeft != "4 min" {
		t.Errorf("downloading row = %+v", q)
	}
	r.Status, r.TrackedDownloadStatus, r.TrackedDownloadState, r.SizeLeft = "completed", "warning", "importPending", 0
	r.StatusMessages = []struct {
		Messages []string `json:"messages"`
	}{{Messages: []string{"Found archive file, might need to be extracted"}}}
	if q := row(r); q.State != "warning" || q.Message != "Found archive file, might need to be extracted" || q.Percent != 100 {
		t.Errorf("warning row = %+v", q)
	}
	for in, want := range map[string]string{"00:00:30": "under a minute", "01:04:12": "1 hr 4 min", "1.02:00:00": "1 day 2 hr", "": ""} {
		if got := humanLeft(in); got != want {
			t.Errorf("humanLeft(%q) = %q, want %q", in, got, want)
		}
	}
	if namedIn("Indexers unavailable due to failures: NZBgeek, DrunkenSlug") != 2 {
		t.Error("namedIn")
	}
	if !sameDrive(7179295580160, 7179300000000) || sameDrive(7276949504, 7179295580160) {
		t.Error("sameDrive")
	}
	if !within("/mnt/media/series", "/mnt/media") || within("/mnt/mediax", "/mnt/media") || !within("/x", "/") {
		t.Error("within")
	}
}
