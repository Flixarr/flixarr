// Package arr talks to Radarr and Sonarr through their v3 API.
package arr

import (
	"bytes"
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

// Kinds Flixarr connects to, with the app name each one reports.
var Kinds = map[string]string{"radarr": "Radarr", "sonarr": "Sonarr"}

// ErrNotFound: the server answered, but the movie or series asked for isn't on it (anymore).
var ErrNotFound = errors.New("not found (404)")

// missingItem tells "no such movie/series" (a JSON problem reply with status 404) from a wrong address or
// URL base, which gets an empty or non-JSON 404.
func missingItem(body []byte) bool {
	var p struct {
		Status int `json:"status"`
	}
	return json.Unmarshal(body, &p) == nil && p.Status == http.StatusNotFound
}

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
	AppURL   string    `json:"applicationUrl,omitempty"` // Settings → General → Host → Application URL, when set
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
		return "", errors.New("Address is required")
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", errors.New("Invalid address (e.g. http://192.168.1.10:7878)")
	}
	u.Path = strings.TrimSuffix(strings.TrimRight(u.Path, "/"), "/api/v3")
	u.Path = strings.TrimSuffix(u.Path, "/api")
	u.RawQuery, u.Fragment = "", ""
	return strings.TrimRight(u.String(), "/"), nil
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	return c.do(ctx, http.MethodGet, path, nil, out)
}

// Post, Put and Get are for packages that change Radarr/Sonarr (quality profiles); the body is sent as JSON.
func (c *Client) Get(ctx context.Context, path string, out any) error {
	return c.do(ctx, http.MethodGet, path, nil, out)
}
func (c *Client) Post(ctx context.Context, path string, body, out any) error {
	return c.do(ctx, http.MethodPost, path, body, out)
}
func (c *Client) Put(ctx context.Context, path string, body, out any) error {
	return c.do(ctx, http.MethodPut, path, body, out)
}

// Delete removes something, with an optional body (bulk deletes take a list of IDs).
func (c *Client) Delete(ctx context.Context, path string, body ...any) error {
	var in any
	if len(body) > 0 {
		in = body[0]
	}
	return c.do(ctx, http.MethodDelete, path, in, nil)
}

func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	var rd io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.URL+"/api/v3/"+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("X-Api-Key", c.APIKey)
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	hc := c.HTTP
	if hc == nil {
		hc = http.DefaultClient
	}
	res, err := hc.Do(req)
	if err != nil {
		return errors.New("no response") // down, unreachable or too slow
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	switch {
	case res.StatusCode == http.StatusUnauthorized:
		return errors.New("API key rejected (401)")
	case res.StatusCode == http.StatusNotFound && method == http.MethodGet && missingItem(body):
		return ErrNotFound
	case res.StatusCode == http.StatusNotFound && method == http.MethodGet:
		return fmt.Errorf("%s isn't at this address (404)", Kinds[c.Kind])
	case res.StatusCode >= 300:
		return &StatusError{Code: res.StatusCode, Body: firstMessage(body)}
	}
	if out == nil || len(body) == 0 {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("%s isn't at this address", Kinds[c.Kind])
	}
	return nil
}

// StatusError is an error answer from Radarr/Sonarr, with its own message when it gave one.
type StatusError struct {
	Code int
	Body string
}

func (e *StatusError) Error() string {
	if e.Body != "" {
		return fmt.Sprintf("%s (%d)", e.Body, e.Code)
	}
	return fmt.Sprintf("error (%d)", e.Code)
}

// firstMessage pulls a readable message out of an error body: Radarr/Sonarr answer validation
// failures with a list of {propertyName, errorMessage}.
func firstMessage(b []byte) string {
	var list []struct {
		ErrorMessage string `json:"errorMessage"`
	}
	if json.Unmarshal(b, &list) == nil && len(list) > 0 {
		return list[0].ErrorMessage
	}
	var one struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(b, &one) == nil {
		return one.Message
	}
	return ""
}

// Test checks the connection and returns the version, quality profiles and root folders.
func (c *Client) Test(ctx context.Context) (Info, error) {
	var st Status
	if err := c.get(ctx, "system/status", &st); err != nil {
		return Info{}, err
	}
	if want := Kinds[c.Kind]; !strings.EqualFold(st.AppName, want) {
		if st.AppName == "" {
			return Info{}, fmt.Errorf("%s isn't at this address", want)
		}
		return Info{}, fmt.Errorf("This is %s, not %s", st.AppName, want)
	}
	info := Info{Version: st.Version, Name: st.InstanceName, Profiles: []Profile{}, Folders: []Folder{}}
	if err := c.get(ctx, "qualityprofile", &info.Profiles); err != nil {
		return Info{}, err
	}
	if err := c.get(ctx, "rootfolder", &info.Folders); err != nil {
		return Info{}, err
	}
	// The address people open it at, if the owner filled it in. Optional; a failure here doesn't fail the test.
	var host struct {
		ApplicationURL string `json:"applicationUrl"`
	}
	if c.get(ctx, "config/host", &host) == nil {
		info.AppURL = strings.TrimRight(strings.TrimSpace(host.ApplicationURL), "/")
	}
	return info, nil
}
