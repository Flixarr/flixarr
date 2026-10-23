// Package plex talks to plex.tv: the PIN sign-in flow and the signed-in account's profile.
package plex

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// tvAPI is plex.tv's address (changed in tests).
var tvAPI = "https://plex.tv/api/v2"

// Client identifies this Flixarr install to plex.tv. ClientID must stay stable for the install.
type Client struct {
	ClientID string
	Product  string
	Version  string
	HTTP     *http.Client
}

// New returns a plex.tv client for this install.
func New(clientID, version string) *Client {
	return &Client{ClientID: clientID, Product: "Flixarr", Version: version, HTTP: &http.Client{Timeout: 15 * time.Second}}
}

func (c *Client) do(ctx context.Context, method, path, token string, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, tvAPI+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Plex-Product", c.Product)
	req.Header.Set("X-Plex-Version", c.Version)
	req.Header.Set("X-Plex-Client-Identifier", c.ClientID)
	if token != "" {
		req.Header.Set("X-Plex-Token", token)
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		return fmt.Errorf("plex.tv %s %s: %s", method, path, res.Status)
	}
	return json.NewDecoder(res.Body).Decode(out)
}

// Pin is a sign-in PIN. AuthToken is filled in once the person approves it on plex.tv.
type Pin struct {
	ID        int64  `json:"id"`
	Code      string `json:"code"`
	AuthToken string `json:"authToken"`
}

// CreatePin starts a sign-in.
func (c *Client) CreatePin(ctx context.Context) (*Pin, error) {
	p := &Pin{}
	return p, c.do(ctx, http.MethodPost, "/pins?strong=true", "", p)
}

// CheckPin returns the PIN's current state; AuthToken is empty until approved.
func (c *Client) CheckPin(ctx context.Context, id int64) (*Pin, error) {
	p := &Pin{}
	return p, c.do(ctx, http.MethodGet, fmt.Sprintf("/pins/%d", id), "", p)
}

// AuthURL is the plex.tv page where the person approves the PIN.
func (c *Client) AuthURL(code string) string {
	q := url.Values{}
	q.Set("clientID", c.ClientID)
	q.Set("code", code)
	q.Set("context[device][product]", c.Product)
	return "https://app.plex.tv/auth#?" + q.Encode()
}

// Account is the signed-in plex.tv user.
type Account struct {
	ID       int64  `json:"id"`
	UUID     string `json:"uuid"`
	Username string `json:"username"`
	Title    string `json:"title"`
	Email    string `json:"email"`
	Thumb    string `json:"thumb"`
}

// Me returns the account that owns the token.
func (c *Client) Me(ctx context.Context, token string) (*Account, error) {
	a := &Account{}
	return a, c.do(ctx, http.MethodGet, "/user", token, a)
}
