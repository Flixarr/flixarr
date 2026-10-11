// Package arr talks to Radarr and Sonarr through their v3 API.
package arr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Kinds Flixarr connects to, with the app name each one reports.
var Kinds = map[string]string{"radarr": "Radarr", "sonarr": "Sonarr"}

// Status is what system/status reports.
type Status struct {
	AppName      string `json:"appName"`
	InstanceName string `json:"instanceName"`
	Version      string `json:"version"`
}

// Profile is a quality profile.
type Profile struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Folder is a root folder.
type Folder struct {
	ID         int    `json:"id"`
	Path       string `json:"path"`
	FreeSpace  int64  `json:"freeSpace"`
	Accessible bool   `json:"accessible"`
}

// Info is everything the Settings screen needs after a successful test.
type Info struct {
	Version  string    `json:"version"`
	Name     string    `json:"name"`
	Profiles []Profile `json:"profiles"`
	Folders  []Folder  `json:"folders"`
}

// Client is one Radarr or Sonarr server.
type Client struct {
	Kind   string // "radarr" or "sonarr"
	URL    string // normalized, e.g. http://10.0.0.5:7878 or https://example.com/radarr
	APIKey string
	HTTP   *http.Client
}

// NormalizeURL adds http:// when no scheme is given and drops a trailing slash and any /api suffix.
func NormalizeURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("enter the address")
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", errors.New("that address doesn't look right; it should be like http://192.168.1.10:7878")
	}
	u.Path = strings.TrimSuffix(strings.TrimRight(u.Path, "/"), "/api/v3")
	u.Path = strings.TrimSuffix(u.Path, "/api")
	u.RawQuery, u.Fragment = "", ""
	return strings.TrimRight(u.String(), "/"), nil
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.URL+"/api/v3/"+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Api-Key", c.APIKey)
	req.Header.Set("Accept", "application/json")
	hc := c.HTTP
	if hc == nil {
		hc = http.DefaultClient
	}
	res, err := hc.Do(req)
	if err != nil {
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			return errors.New("no answer within 10 seconds; check the address and that it's running")
		}
		return errors.New("couldn't reach it; check the address and that it's running")
	}
	defer res.Body.Close()
	switch {
	case res.StatusCode == http.StatusUnauthorized:
		return errors.New("the API key was refused")
	case res.StatusCode == http.StatusNotFound:
		return fmt.Errorf("this address doesn't answer like %s; check the address and any URL base", Kinds[c.Kind])
	case res.StatusCode >= 300:
		return fmt.Errorf("it answered with an error (%d)", res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("this address doesn't answer like %s", Kinds[c.Kind])
	}
	return nil
}

// Test checks the connection and returns the version, quality profiles and root folders.
func (c *Client) Test(ctx context.Context) (Info, error) {
	var st Status
	if err := c.get(ctx, "system/status", &st); err != nil {
		return Info{}, err
	}
	if want := Kinds[c.Kind]; !strings.EqualFold(st.AppName, want) {
		if st.AppName == "" {
			return Info{}, fmt.Errorf("this address doesn't answer like %s", want)
		}
		return Info{}, fmt.Errorf("that's %s, not %s", st.AppName, want)
	}
	info := Info{Version: st.Version, Name: st.InstanceName, Profiles: []Profile{}, Folders: []Folder{}}
	if err := c.get(ctx, "qualityprofile", &info.Profiles); err != nil {
		return Info{}, err
	}
	if err := c.get(ctx, "rootfolder", &info.Folders); err != nil {
		return Info{}, err
	}
	return info, nil
}
