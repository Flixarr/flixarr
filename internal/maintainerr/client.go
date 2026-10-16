// Package maintainerr talks to Maintainerr, which removes titles from Plex, Radarr and Sonarr by rules:
// a title that matches goes into a "leaving" collection and is deleted after the collection's number of
// days. Flixarr reads those collections (leaving dates, freed space), keeps titles out of them on request
// (exclusions and postponing), and adds itself to Maintainerr's notifications to hear about changes.
package maintainerr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DefaultPort is Maintainerr's port.
const DefaultPort = 6246

// Client calls one Maintainerr.
type Client struct {
	URL    string // e.g. http://192.168.1.10:6246
	APIKey string // sent as X-Api-Key when set (Maintainerr doesn't ask for one today)
	HTTP   *http.Client
}

func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, _ := json.Marshal(in)
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.URL, "/")+"/api"+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.APIKey != "" {
		req.Header.Set("X-Api-Key", c.APIKey)
	}
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 20 * time.Second}
	}
	res, err := hc.Do(req)
	if err != nil {
		return errors.New("couldn't reach it; check the address and that it's running")
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(res.Body, 16<<20))
	if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden {
		return errors.New("Maintainerr refused the API key")
	}
	if res.StatusCode >= 300 {
		var m struct {
			Message any `json:"message"`
		}
		json.Unmarshal(b, &m)
		if s, ok := m.Message.(string); ok && s != "" {
			return fmt.Errorf("Maintainerr answered %d: %s", res.StatusCode, s)
		}
		return fmt.Errorf("Maintainerr answered %d", res.StatusCode)
	}
	if out == nil || len(b) == 0 {
		return nil
	}
	if err := json.Unmarshal(b, out); err != nil {
		return errors.New("this address doesn't answer like Maintainerr")
	}
	return nil
}

// Status is what the status call says.
type Status struct {
	Version string `json:"version"`
}

// Status checks the connection and returns Maintainerr's version.
func (c *Client) Status(ctx context.Context) (Status, error) {
	var s Status
	err := c.do(ctx, http.MethodGet, "/app/status", nil, &s)
	if err == nil && s.Version == "" {
		err = errors.New("this address doesn't answer like Maintainerr")
	}
	return s, err
}

// HandlerSchedule is when Maintainerr removes titles whose time is up (a cron expression; its default
// is daily). Titles leave on its first run after their days in the collection have passed.
func (c *Client) HandlerSchedule(ctx context.Context) string {
	var s struct {
		Cron string `json:"collection_handler_job_cron"`
	}
	if c.do(ctx, http.MethodGet, "/settings", nil, &s) != nil {
		return ""
	}
	return s.Cron
}

// Collection is one rule's "leaving" collection.
type Collection struct {
	ID              int     `json:"id"`
	Title           string  `json:"title"`
	Type            string  `json:"type"` // movie | show | season | episode
	Active          bool    `json:"isActive"`
	DeleteAfterDays int     `json:"deleteAfterDays"`
	Handled         int     `json:"handledMediaAmount"`
	HandledBytes    float64 `json:"handledMediaSizeBytes"`
	MediaCount      int     `json:"mediaCount"`
	Media           []Media `json:"-"`
}

// Media is a title in a collection.
type Media struct {
	PlexID string    `json:"mediaServerId"` // the Plex rating key
	TmdbID int       `json:"tmdbId"`
	Added  time.Time `json:"-"`
	Raw    string    `json:"addDate"`
}

// Collections lists the collections with all their titles.
func (c *Client) Collections(ctx context.Context) ([]Collection, error) {
	var list []Collection
	if err := c.do(ctx, http.MethodGet, "/collections", nil, &list); err != nil {
		return nil, err
	}
	for i := range list {
		for page := 1; page < 100; page++ {
			var res struct {
				Total int     `json:"totalSize"`
				Items []Media `json:"items"`
			}
			if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/collections/media/%d/content/%d?size=500", list[i].ID, page), nil, &res); err != nil {
				return nil, err
			}
			for _, m := range res.Items {
				m.Added = parseDate(m.Raw)
				list[i].Media = append(list[i].Media, m)
			}
			if len(res.Items) == 0 || len(list[i].Media) >= res.Total {
				break
			}
		}
	}
	return list, nil
}

// parseDate reads Maintainerr's dates ("2026-10-13T04:00:00.000Z", or "2026-10-13 04:00:00.000" in UTC).
func parseDate(s string) time.Time {
	for _, f := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.000", "2006-01-02 15:04:05"} {
		if t, err := time.Parse(f, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

// Exclude keeps a title (by Plex rating key) out of every rule, so it's never deleted.
func (c *Client) Exclude(ctx context.Context, plexID string) error {
	return c.exclusion(ctx, plexID, 0)
}

// Unexclude lets the rules see a title again.
func (c *Client) Unexclude(ctx context.Context, plexID string) error {
	return c.exclusion(ctx, plexID, 1)
}

func (c *Client) exclusion(ctx context.Context, plexID string, action int) error {
	var res struct {
		Code   int    `json:"code"`
		Result string `json:"result"`
	}
	if err := c.do(ctx, http.MethodPost, "/rules/exclusion", map[string]any{"mediaId": plexID, "action": action}, &res); err != nil {
		return err
	}
	if res.Code == 0 && res.Result != "" && !strings.Contains(strings.ToLower(res.Result), "success") {
		return fmt.Errorf("Maintainerr said: %s", res.Result)
	}
	return nil
}

// Postpone pushes a title's deletion back by days (from today).
func (c *Client) Postpone(ctx context.Context, collectionID int, plexID string, days int) error {
	return c.do(ctx, http.MethodPost, "/collections/media/postpone", map[string]any{"collectionId": collectionID, "mediaId": plexID, "days": days}, nil)
}

// WebhookName is the name Flixarr's entry has in Maintainerr's notifications.
const WebhookName = "Flixarr"

// AddWebhook adds Flixarr to Maintainerr's notifications (a webhook for titles added to or removed from a
// collection, about to be handled, and handled) and connects it to every rule. Done again, it replaces
// Flixarr's old entry.
func (c *Client) AddWebhook(ctx context.Context, url string) error {
	var configs []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	if err := c.do(ctx, http.MethodGet, "/notifications/configurations", nil, &configs); err != nil {
		return err
	}
	for _, cfg := range configs {
		if cfg.Name == WebhookName {
			c.do(ctx, http.MethodDelete, fmt.Sprintf("/notifications/configuration/%d", cfg.ID), nil, nil)
		}
	}
	payload := map[string]any{"event": "{{notification_type}}", "subject": "{{subject}}", "message": "{{message}}"} // an object, not text
	in := map[string]any{"agent": "webhook", "name": WebhookName, "enabled": true, "types": []int{2, 4, 8, 16}, "aboutScale": 3,
		"options": map[string]any{"webhookUrl": url, "jsonPayload": payload, "authHeader": ""}}
	if err := c.do(ctx, http.MethodPost, "/notifications/configuration/add", in, nil); err != nil {
		return err
	}
	if err := c.do(ctx, http.MethodGet, "/notifications/configurations", nil, &configs); err != nil {
		return err
	}
	id := 0
	for _, cfg := range configs {
		if cfg.Name == WebhookName {
			id = cfg.ID
		}
	}
	if id == 0 {
		return errors.New("Maintainerr didn't keep the webhook")
	}
	var rules []struct {
		ID int `json:"id"`
	}
	if err := c.do(ctx, http.MethodGet, "/rules", nil, &rules); err != nil {
		return err
	}
	for _, r := range rules {
		if err := c.do(ctx, http.MethodPost, "/notifications/configuration/connect", map[string]any{"rulegroupId": r.ID, "notificationId": id}, nil); err != nil {
			return err
		}
	}
	return nil
}

// HasWebhook says whether Flixarr's webhook is in Maintainerr's notifications.
func (c *Client) HasWebhook(ctx context.Context) bool {
	var configs []struct {
		Name    string `json:"name"`
		Enabled bool   `json:"enabled"`
	}
	if c.do(ctx, http.MethodGet, "/notifications/configurations", nil, &configs) != nil {
		return false
	}
	for _, cfg := range configs {
		if cfg.Name == WebhookName && cfg.Enabled {
			return true
		}
	}
	return false
}
