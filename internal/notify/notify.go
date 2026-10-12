// Package notify sends Flixarr events (requests submitted, approved, declined, available, failed) out
// through the channels set up in Settings → Notifications: Discord and webhooks for now.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/Flixarr/flixarr/internal/store"
)

// Events, in the order Settings shows them. The issue events arrive with Issues.
var Events = []string{"request_submitted", "request_approved", "request_declined", "request_available", "request_failed", "issue_reported", "issue_resolved"}

// Channels Flixarr can send through.
var Channels = []string{"discord", "webhook"}

// Config is one channel's settings. Secret fields (the Discord webhook URL, the webhook's address and
// Authorization header) are stored encrypted and never sent back to the browser.
type Config struct {
	On     bool     `json:"on"`
	Events []string `json:"events"`
	// Discord
	WebhookURL  string `json:"webhookUrl,omitempty"`
	BotName     string `json:"botName,omitempty"`
	Poster      bool   `json:"poster"`
	MentionHere bool   `json:"mentionHere"`
	// Webhook
	URL    string `json:"url,omitempty"`
	Method string `json:"method,omitempty"`
	Auth   string `json:"auth,omitempty"`
}

var defaults = map[string]Config{
	"discord": {Events: []string{"request_submitted", "request_approved", "request_declined", "request_available", "request_failed"}, BotName: "Flixarr", Poster: true},
	"webhook": {Events: []string{"request_available", "request_failed"}, Method: "POST"},
}

// Notifier loads the channel settings and sends.
type Notifier struct {
	Store *store.Store
	Log   *slog.Logger
	HTTP  *http.Client
}

func key(ch string) string { return "notify_" + ch }

// Load returns a channel's settings with secrets decrypted.
func (n *Notifier) Load(ch string) Config {
	c := defaults[ch]
	if raw := n.Store.SecretSetting(key(ch), ""); raw != "" {
		json.Unmarshal([]byte(raw), &c)
	}
	if c.Events == nil {
		c.Events = []string{}
	}
	return c
}

// Save stores a channel's settings (encrypted as a whole: they hold webhook addresses and tokens).
func (n *Notifier) Save(ch string, c Config) error {
	if !slices.Contains(Channels, ch) {
		return errors.New("unknown channel")
	}
	if err := c.validate(ch); err != nil {
		return err
	}
	b, _ := json.Marshal(c)
	return n.Store.SetSecretSetting(key(ch), string(b))
}

func (c Config) validate(ch string) error {
	check := func(raw, what string) error {
		if raw == "" {
			if c.On {
				return fmt.Errorf("enter the %s first", what)
			}
			return nil
		}
		u, err := url.Parse(raw)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			return fmt.Errorf("that %s doesn't look right", what)
		}
		return nil
	}
	switch ch {
	case "discord":
		if err := check(c.WebhookURL, "Discord webhook URL"); err != nil {
			return err
		}
		if c.WebhookURL != "" && !strings.Contains(c.WebhookURL, "/api/webhooks/") {
			return errors.New("that isn't a Discord webhook URL; copy it from the channel's Integrations, Webhooks")
		}
	case "webhook":
		if c.Method != "POST" && c.Method != "PUT" {
			return errors.New("the method has to be POST or PUT")
		}
		return check(c.URL, "webhook URL")
	}
	return nil
}

// Message is what an event says.
type Message struct {
	Event   string
	Title   string // "The Matrix (1999)"
	Text    string // "Requested by eryn"
	Status  string
	User    string
	Type    string // movie | series
	TmdbID  int
	Poster  string // TMDB path
	Seasons string
	Error   string
	Link    string // into Flixarr, when an application URL is set
	Request int64
}

var eventText = map[string]struct {
	head  string
	color int
}{
	"request_submitted": {"New request", 0xf97316},
	"request_approved":  {"Request approved", 0x3b82f6},
	"request_declined":  {"Request declined", 0x6b7280},
	"request_available": {"Now available", 0x22c55e},
	"request_failed":    {"Request failed", 0xef4444},
	"test":              {"Test notification", 0xf97316},
}

// FromRequest builds the message for a request event.
func (n *Notifier) FromRequest(event string, r store.Request) Message {
	user := r.UserName
	if user == "" {
		if u, err := n.Store.UserByID(r.UserID); err == nil {
			user = u.Username
		}
	}
	title := r.Title
	if r.Year != "" {
		title += " (" + r.Year + ")"
	}
	m := Message{Event: event, Title: title, User: user, Status: r.Status, Type: r.Type, TmdbID: r.TmdbID, Poster: r.Poster, Error: r.Error, Request: r.ID}
	if r.Type == "series" {
		if r.Mode == "all" || len(r.Seasons) == 0 {
			m.Seasons = "Whole show"
		} else {
			parts := []string{}
			for _, s := range r.Seasons {
				parts = append(parts, fmt.Sprint(s))
			}
			m.Seasons = "Seasons " + strings.Join(parts, ", ")
		}
	}
	switch event {
	case "request_submitted":
		m.Text = "Requested by " + user
		if r.Status == "pending" {
			m.Text += ", waiting for approval"
		}
	case "request_approved":
		m.Text = "Requested by " + user + ", now on its way"
	case "request_declined":
		m.Text = "Requested by " + user
	case "request_available":
		m.Text = "Ready to watch in Plex. Requested by " + user
	case "request_failed":
		m.Text = "Requested by " + user + ". " + r.Error
	}
	if base := n.Store.Setting("app_url", ""); base != "" {
		m.Link = fmt.Sprintf("%s/%s/%d", base, r.Type, r.TmdbID)
	}
	return m
}

// Send delivers a message to every channel that's on and has the event, in the background.
func (n *Notifier) Send(m Message) {
	for _, ch := range Channels {
		c := n.Load(ch)
		if !c.On || !slices.Contains(c.Events, m.Event) {
			continue
		}
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			if err := n.deliver(ctx, ch, c, m); err != nil {
				n.Log.Warn("notification not sent", "channel", ch, "event", m.Event, "err", err)
			}
		}()
	}
}

// Test sends a test message through a channel with the given (maybe unsaved) settings.
func (n *Notifier) Test(ctx context.Context, ch string, c Config) error {
	c.On = true
	if err := c.validate(ch); err != nil {
		return err
	}
	m := Message{Event: "test", Title: "Flixarr is connected", Text: "Notifications from Flixarr will show up here.", Link: n.Store.Setting("app_url", "")}
	return n.deliver(ctx, ch, c, m)
}

func (n *Notifier) deliver(ctx context.Context, ch string, c Config, m Message) error {
	switch ch {
	case "discord":
		return n.post(ctx, http.MethodPost, c.WebhookURL, "", discordBody(c, m))
	case "webhook":
		return n.post(ctx, c.Method, c.URL, c.Auth, webhookBody(m))
	}
	return errors.New("unknown channel")
}

func discordBody(c Config, m Message) map[string]any {
	head := eventText[m.Event]
	embed := map[string]any{"title": m.Title, "description": m.Text, "color": head.color, "author": map[string]any{"name": head.head},
		"footer": map[string]any{"text": "Flixarr"}, "timestamp": time.Now().UTC().Format(time.RFC3339)}
	if m.Link != "" {
		embed["url"] = m.Link
	}
	fields := []any{}
	if m.Seasons != "" {
		fields = append(fields, map[string]any{"name": "Seasons", "value": m.Seasons, "inline": true})
	}
	if len(fields) > 0 {
		embed["fields"] = fields
	}
	if c.Poster && m.Poster != "" {
		embed["thumbnail"] = map[string]any{"url": "https://image.tmdb.org/t/p/w342" + m.Poster}
	}
	body := map[string]any{"username": c.BotName, "embeds": []any{embed}, "allowed_mentions": map[string]any{"parse": []string{}}}
	if c.BotName == "" {
		body["username"] = "Flixarr"
	}
	if c.MentionHere && m.Event == "request_submitted" {
		body["content"] = "@here"
		body["allowed_mentions"] = map[string]any{"parse": []string{"everyone"}}
	}
	return body
}

func webhookBody(m Message) map[string]any {
	out := map[string]any{"event": m.Event, "title": m.Title, "message": m.Text}
	if m.Event != "test" {
		out["media"] = map[string]any{"type": m.Type, "tmdbId": m.TmdbID, "poster": m.Poster}
		out["request"] = map[string]any{"id": m.Request, "status": m.Status, "user": m.User, "seasons": m.Seasons, "error": m.Error}
	}
	if m.Link != "" {
		out["url"] = m.Link
	}
	return out
}

func (n *Notifier) post(ctx context.Context, method, to, auth string, body any) error {
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, method, to, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Flixarr")
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	hc := n.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second}
	}
	res, err := hc.Do(req)
	if err != nil {
		return errors.New("couldn't reach it")
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(res.Body, 300))
		return fmt.Errorf("it answered %s %s", res.Status, strings.TrimSpace(string(msg)))
	}
	return nil
}
