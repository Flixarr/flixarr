// Package seerr brings users, requests, issues and the Radarr/Sonarr connections over from Seerr,
// Overseerr or Jellyseerr (Settings → Import). It reads their API, which is the same across all three
// and across versions, and never changes anything there.
package seerr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client reads one Seerr server.
type Client struct {
	URL  string // e.g. http://192.168.1.10:5055
	Key  string // Seerr's API key (Settings, General)
	HTTP *http.Client
}

// NewClient checks the address and returns a client for it.
func NewClient(raw, key string) (*Client, error) {
	raw = strings.TrimSpace(raw)
	if raw != "" && !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errors.New("that address doesn't look right")
	}
	if strings.TrimSpace(key) == "" {
		return nil, errors.New("enter the API key")
	}
	base := strings.TrimRight(u.Scheme+"://"+u.Host+u.Path, "/")
	base = strings.TrimSuffix(base, "/api/v1")
	return &Client{URL: base, Key: strings.TrimSpace(key), HTTP: &http.Client{Timeout: 60 * time.Second}}, nil
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.URL+"/api/v1/"+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Api-Key", c.Key)
	req.Header.Set("Accept", "application/json")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return errors.New("couldn't reach Seerr at that address")
	}
	defer res.Body.Close()
	switch {
	case res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden:
		return errors.New("Seerr refused the API key")
	case res.StatusCode == http.StatusNotFound:
		return errors.New("that address answered, but it isn't Seerr")
	case res.StatusCode != http.StatusOK:
		return fmt.Errorf("Seerr answered %s", res.Status)
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 64<<20)).Decode(out); err != nil {
		return errors.New("that address answered, but it isn't Seerr")
	}
	return nil
}

// Status is Seerr's version.
type Status struct {
	Version string `json:"version"`
}

// User is a Seerr user. UserType: 1 Plex, 2 local, 3 Jellyfin, 4 Emby.
type User struct {
	ID              int64  `json:"id"`
	UserType        int    `json:"userType"`
	PlexID          int64  `json:"plexId"`
	PlexUsername    string `json:"plexUsername"`
	Username        string `json:"username"`
	DisplayName     string `json:"displayName"`
	Email           string `json:"email"`
	Avatar          string `json:"avatar"`
	Permissions     int64  `json:"permissions"`
	MovieQuotaLimit int    `json:"movieQuotaLimit"`
	MovieQuotaDays  int    `json:"movieQuotaDays"`
	TvQuotaLimit    int    `json:"tvQuotaLimit"`
	TvQuotaDays     int    `json:"tvQuotaDays"`
	CreatedAt       string `json:"createdAt"`
}

// Name is how the person shows up: their Plex name, else their username, else the display name.
func (u User) Name() string {
	for _, n := range []string{u.PlexUsername, u.Username, u.DisplayName, u.Email} {
		if n = strings.TrimSpace(n); n != "" {
			return n
		}
	}
	return fmt.Sprintf("seerr-%d", u.ID)
}

// Media is the title a request or issue is about. Status: 1 unknown, 2 pending, 3 processing,
// 4 partly available, 5 available, 6 blocklisted, 7 deleted.
type Media struct {
	TmdbID       int    `json:"tmdbId"`
	TvdbID       int    `json:"tvdbId"`
	MediaType    string `json:"mediaType"` // movie | tv
	Status       int    `json:"status"`
	MediaAddedAt string `json:"mediaAddedAt"`
}

// Season is one season of a series request.
type Season struct {
	SeasonNumber int `json:"seasonNumber"`
}

// Request is a Seerr request. Status: 1 pending, 2 approved, 3 declined, 4 failed, 5 completed.
type Request struct {
	ID          int64    `json:"id"`
	Type        string   `json:"type"` // movie | tv
	Status      int      `json:"status"`
	Is4k        bool     `json:"is4k"`
	Media       Media    `json:"media"`
	Seasons     []Season `json:"seasons"`
	RequestedBy User     `json:"requestedBy"`
	ModifiedBy  *User    `json:"modifiedBy"`
	ProfileID   int      `json:"profileId"`
	ProfileName string   `json:"profileName"`
	CreatedAt   string   `json:"createdAt"`
	UpdatedAt   string   `json:"updatedAt"`
}

// Comment is one message on a Seerr issue.
type Comment struct {
	Message   string `json:"message"`
	User      *User  `json:"user"`
	CreatedAt string `json:"createdAt"`
}

// Issue is a Seerr issue. IssueType: 1 video, 2 audio, 3 subtitles, 4 other. Status: 1 open, 2 resolved.
type Issue struct {
	ID             int64     `json:"id"`
	IssueType      int       `json:"issueType"`
	Status         int       `json:"status"`
	ProblemSeason  int       `json:"problemSeason"`
	ProblemEpisode int       `json:"problemEpisode"`
	Media          Media     `json:"media"`
	CreatedBy      User      `json:"createdBy"`
	ModifiedBy     *User     `json:"modifiedBy"`
	Comments       []Comment `json:"comments"`
	CreatedAt      string    `json:"createdAt"`
	UpdatedAt      string    `json:"updatedAt"`
}

// Server is a Radarr or Sonarr connection set up in Seerr.
type Server struct {
	Name                string `json:"name"`
	Hostname            string `json:"hostname"`
	Port                int    `json:"port"`
	APIKey              string `json:"apiKey"`
	UseSsl              bool   `json:"useSsl"`
	BaseURL             string `json:"baseUrl"`
	ExternalURL         string `json:"externalUrl"`
	ActiveProfileID     int    `json:"activeProfileId"`
	ActiveProfileName   string `json:"activeProfileName"`
	ActiveDirectory     string `json:"activeDirectory"`
	Is4k                bool   `json:"is4k"`
	IsDefault           bool   `json:"isDefault"`
	EnableSeasonFolders bool   `json:"enableSeasonFolders"`
	PreventSearch       bool   `json:"preventSearch"`
}

// Address is the server's address as Flixarr stores it.
func (s Server) Address() string {
	scheme := "http"
	if s.UseSsl {
		scheme = "https"
	}
	base := strings.TrimRight(s.BaseURL, "/")
	if base != "" && !strings.HasPrefix(base, "/") {
		base = "/" + base
	}
	return fmt.Sprintf("%s://%s:%d%s", scheme, s.Hostname, s.Port, base)
}

// Data is everything read from Seerr.
type Data struct {
	Version  string
	Users    []User
	Requests []Request
	Issues   []Issue
	Radarr   []Server
	Sonarr   []Server
}

type page[T any] struct {
	PageInfo struct {
		Pages   int `json:"pages"`
		Results int `json:"results"`
	} `json:"pageInfo"`
	Results []T `json:"results"`
}

const take = 100

// all reads every page of a list (path already has its own query, e.g. "request?filter=all").
func all[T any](ctx context.Context, c *Client, path string) ([]T, error) {
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	out := []T{}
	for skip := 0; ; skip += take {
		var p page[T]
		if err := c.get(ctx, fmt.Sprintf("%s%stake=%d&skip=%d", path, sep, take, skip), &p); err != nil {
			return nil, err
		}
		out = append(out, p.Results...)
		if len(p.Results) < take || len(out) >= p.PageInfo.Results {
			return out, nil
		}
	}
}

// Fetch reads everything to import. Issues are read one by one for their comments' authors.
func (c *Client) Fetch(ctx context.Context) (*Data, error) {
	var st Status
	if err := c.get(ctx, "status", &st); err != nil {
		return nil, err
	}
	if st.Version == "" {
		return nil, errors.New("that address answered, but it isn't Seerr")
	}
	d := &Data{Version: st.Version}
	var err error
	if d.Users, err = all[User](ctx, c, "user"); err != nil {
		return nil, err
	}
	if d.Requests, err = all[Request](ctx, c, "request?filter=all&sort=added"); err != nil {
		return nil, err
	}
	list, err := all[Issue](ctx, c, "issue?filter=all&sort=added")
	if err != nil {
		return nil, err
	}
	for _, i := range list {
		var full Issue
		if err := c.get(ctx, fmt.Sprintf("issue/%d", i.ID), &full); err != nil {
			return nil, err
		}
		d.Issues = append(d.Issues, full)
	}
	// Seerr answers these only to admins; a key without them just means no servers come over.
	c.get(ctx, "settings/radarr", &d.Radarr)
	c.get(ctx, "settings/sonarr", &d.Sonarr)
	return d, nil
}
