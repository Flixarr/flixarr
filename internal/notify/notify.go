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

// Events, in the order Settings shows them.
var Events = []string{"request_submitted", "request_approved", "request_declined", "request_available", "request_failed", "issue_reported", "issue_resolved", "server_problem"}

// Channels Flixarr can send through.
var Channels = []string{"discord", "webhook", "email", "push"}

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
	// Email (SMTP): sent to the people involved, see recipients.
	Host     string `json:"host,omitempty"`
	Port     int    `json:"port,omitempty"`
	Security string `json:"security,omitempty"` // starttls | tls | none
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	From     string `json:"from,omitempty"`
	FromName string `json:"fromName,omitempty"`
	TestTo   string `json:"testTo,omitempty"` // where Send test goes (not saved)
	TestUser int64  `json:"-"`                // whose devices the Web Push test goes to
}

var defaults = map[string]Config{
	"discord": {Events: []string{"request_submitted", "request_approved", "request_declined", "request_available", "request_failed", "issue_reported", "issue_resolved", "server_problem"}, BotName: "Flixarr", Poster: true},
	"webhook": {Events: []string{"request_available", "request_failed"}, Method: "POST"},
	"push":    {Events: []string{"request_submitted", "request_approved", "request_declined", "request_available", "request_failed", "issue_reported", "issue_resolved", "server_problem"}},
	"email":   {Events: []string{"request_submitted", "request_approved", "request_declined", "request_available", "request_failed", "issue_reported", "issue_resolved", "server_problem"}, Port: 587, Security: "starttls", FromName: "Flixarr"},
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

// recipients: news about a request or issue goes to the person who made it; new requests go to people
// who manage requests, new issues to people who manage issues. Only people with an email address.
func (n *Notifier) recipients(m Message) []string {
	out := []string{}
	for _, u := range n.peopleOn(m, "email") {
		if u.Email != "" {
			out = append(out, u.Email)
		}
	}
	return out
}

// people is who an event is for (email and Web Push).
func (n *Notifier) people(m Message) []store.User {
	perm := map[string]string{"request_submitted": "manageRequests", "issue_reported": "manageIssues", "server_problem": "admin"}[m.Event]
	if perm == "" {
		if u, err := n.Store.UserByID(m.UserID); err == nil {
			return []store.User{*u}
		}
		return nil
	}
	out := []store.User{}
	rows, _ := n.Store.Users()
	for _, u := range rows {
		if u.Can(perm) && u.ID != m.UserID {
			out = append(out, u.User)
		}
	}
	return out
}

// PersonalChannels are the channels that reach one person (Discord and webhooks go to a shared place).
var PersonalChannels = []string{"email", "push"}

func offKey(userID int64) string { return fmt.Sprintf("notify_off_%d", userID) }

// Off is what a person has switched off: channel → events. Everything is on until they change it.
func (n *Notifier) Off(userID int64) map[string][]string {
	out := map[string][]string{"email": {}, "push": {}}
	json.Unmarshal([]byte(n.Store.Setting(offKey(userID), "")), &out)
	return out
}

// SetOff saves what a person has switched off (unknown channels and events are dropped).
func (n *Notifier) SetOff(userID int64, off map[string][]string) error {
	clean := map[string][]string{}
	for _, ch := range PersonalChannels {
		clean[ch] = []string{}
		for _, e := range off[ch] {
			if slices.Contains(Events, e) && !slices.Contains(clean[ch], e) {
				clean[ch] = append(clean[ch], e)
			}
		}
	}
	b, _ := json.Marshal(clean)
	return n.Store.SetSetting(offKey(userID), string(b))
}

// EventsFor are the events a person can get: news about their own requests and issues, plus new
// requests, new issues and server problems for the people who handle them.
func EventsFor(u *store.User) []string {
	out := []string{}
	for _, e := range Events {
		switch e {
		case "request_submitted":
			if !u.Can("manageRequests") {
				continue
			}
		case "issue_reported":
			if !u.Can("manageIssues") {
				continue
			}
		case "server_problem":
			if !u.Can("admin") {
				continue
			}
		}
		out = append(out, e)
	}
	return out
}

// peopleOn is who an event goes to on a personal channel, leaving out anyone who switched it off.
func (n *Notifier) peopleOn(m Message, ch string) []store.User {
	out := []store.User{}
	for _, u := range n.people(m) {
		if !slices.Contains(n.Off(u.ID)[ch], m.Event) {
			out = append(out, u)
		}
	}
	return out
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
	case "email":
		if c.Security != "starttls" && c.Security != "tls" && c.Security != "none" {
			return errors.New("pick how to connect: STARTTLS, SSL/TLS or none")
		}
		if c.On && (strings.TrimSpace(c.Host) == "" || c.Port <= 0 || c.Port > 65535) {
			return errors.New("enter the SMTP server and port first")
		}
		if c.On && !strings.Contains(c.From, "@") {
			return errors.New("enter the From address")
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
	Request int64  // the request (or issue) ID
	UserID  int64  // who asked (or reported): emails about their request go to them
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
	"issue_reported":    {"Issue reported", 0xeab308},
	"issue_resolved":    {"Issue resolved", 0x22c55e},
	"server_problem":    {"Radarr or Sonarr problem", 0xef4444},
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
	m := Message{Event: event, Title: title, User: user, Status: r.Status, Type: r.Type, TmdbID: r.TmdbID, Poster: r.Poster, Error: r.Error, Request: r.ID, UserID: r.UserID}
	if r.Type == "series" {
		if r.Mode == "episodes" {
			m.Seasons = fmt.Sprintf("%d episodes", len(r.Episodes))
			if len(r.Episodes) == 1 {
				m.Seasons = fmt.Sprintf("Season %d, episode %d", r.Episodes[0][0], r.Episodes[0][1])
			}
		} else if r.Mode == "all" || len(r.Seasons) == 0 {
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
		if r.Error != "" {
			m.Text += ". Reason: " + r.Error
		}
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

// FromIssue builds the message for an issue event.
func (n *Notifier) FromIssue(event string, i store.Issue) Message {
	title := i.Title
	if i.Year != "" {
		title += " (" + i.Year + ")"
	}
	what := i.Kind
	if i.Sub != "" {
		what += ": " + i.Sub
	}
	where := PartsText(i.Parts)
	m := Message{Event: event, Title: title, User: i.UserName, Status: i.Status, Type: i.Type, TmdbID: i.TmdbID, Poster: i.Poster, Seasons: where, Request: i.ID, UserID: i.UserID}
	if event == "issue_reported" {
		m.Text = what + ". Reported by " + i.UserName
		if i.Note != "" {
			m.Text += ": " + i.Note
		}
	} else {
		m.Text = what + ", reported by " + i.UserName + ", is fixed"
	}
	if base := n.Store.Setting("app_url", ""); base != "" {
		m.Link = base + "/issues"
	}
	return m
}

// FromServer builds the message for a Radarr/Sonarr problem (admins only).
func (n *Notifier) FromServer(name, detail string) Message {
	m := Message{Event: "server_problem", Title: name, Text: detail}
	if base := n.Store.Setting("app_url", ""); base != "" {
		m.Link = base + "/settings#services"
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
	if ch == "push" {
		if subs, _ := n.Store.PushSubs(c.TestUser); len(subs) == 0 {
			return errors.New("turn on notifications for this device in your Profile first")
		}
	}
	if ch == "email" && !strings.Contains(c.TestTo, "@") {
		return errors.New("there's no email address to send the test to")
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
	case "email":
		to := n.recipients(m)
		if m.Event == "test" {
			to = []string{c.TestTo}
		}
		if len(to) == 0 {
			return nil
		}
		return sendMail(ctx, c, to, m)
	case "push":
		ids := []int64{}
		for _, u := range n.peopleOn(m, "push") {
			ids = append(ids, u.ID)
		}
		if m.Event == "test" {
			ids = []int64{c.TestUser}
		}
		return n.push(ctx, ids, m)
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
		name := "Seasons"
		if strings.HasPrefix(m.Event, "issue_") {
			name = "Where"
		}
		fields = append(fields, map[string]any{"name": name, "value": m.Seasons, "inline": true})
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
	if m.Event == "server_problem" {
		out["server"] = map[string]any{"name": m.Title, "problem": m.Text}
	} else if m.Event != "test" {
		out["media"] = map[string]any{"type": m.Type, "tmdbId": m.TmdbID, "poster": m.Poster}
		if strings.HasPrefix(m.Event, "issue_") {
			out["issue"] = map[string]any{"id": m.Request, "status": m.Status, "user": m.User, "where": m.Seasons}
		} else {
			out["request"] = map[string]any{"id": m.Request, "status": m.Status, "user": m.User, "seasons": m.Seasons, "error": m.Error}
		}
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

// PartsText reads [season, episode] pairs as "S2E3, S2E4, Season 3" (episode 0 = the whole season).
func PartsText(parts [][2]int) string {
	out := []string{}
	for _, p := range parts {
		if p[1] == 0 {
			out = append(out, fmt.Sprintf("Season %d", p[0]))
		} else {
			out = append(out, fmt.Sprintf("S%dE%d", p[0], p[1]))
		}
	}
	if len(out) > 6 {
		out = append(out[:6], fmt.Sprintf("%d more", len(out)-6))
	}
	return strings.Join(out, ", ")
}
