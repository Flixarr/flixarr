// Package notify sends Flixarr events (requests submitted, approved, declined, available, failed) out
// through the channels set up in Settings → Notifications. Each service (Discord, email, webhooks…) can
// have any number of channels, each with its own settings and events.
package notify

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Flixarr/flixarr/internal/requests"
	"github.com/Flixarr/flixarr/internal/store"
	"github.com/Flixarr/flixarr/internal/testinstall"
)

// Events, in the order Settings shows them.
var Events = []string{"request_submitted", "request_approved", "request_declined", "request_available", "request_failed", "issue_reported", "issue_resolved", "server_problem", "request_leaving"}

// Services Flixarr can send through.
var Services = []string{"discord", "webhook", "email", "push", "telegram", "slack", "pushover", "gotify", "ntfy", "pushbullet"}

// Labels are the services' names, used to name channels made from older settings.
var Labels = map[string]string{"discord": "Discord", "webhook": "Webhook", "email": "Email", "push": "Web Push", "telegram": "Telegram",
	"slack": "Slack", "pushover": "Pushover", "gotify": "Gotify", "ntfy": "ntfy", "pushbullet": "Pushbullet"}

// Channel is one set-up destination: a service, a name and its settings.
type Channel struct {
	ID      string `json:"id"`
	Service string `json:"service"`
	Name    string `json:"name"`
	Config
}

// Config is one channel's settings. Secret fields (the Discord webhook URL, the webhook's address and
// Authorization header) are stored encrypted and never sent back to the browser.
type Config struct {
	On     bool     `json:"on"`
	Events []string `json:"events"`
	// FromDev: a dev install sends this channel's notifications too (#136). Off by default, so a dev
	// install never doubles a real install's notifications by accident; ignored on a real install.
	FromDev bool `json:"fromDev"`
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
	// Telegram, Slack, Pushover, Gotify, ntfy, Pushbullet (see services.go)
	Token    string `json:"token,omitempty"`  // the service's token (Slack: its webhook URL)
	Target   string `json:"target,omitempty"` // Telegram chat ID, Pushover user key, ntfy topic
	Server   string `json:"server,omitempty"` // Gotify / ntfy address
	TestTo   string `json:"testTo,omitempty"` // where Send test goes (not saved)
	TestUser int64  `json:"-"`                // whose devices the Web Push test goes to
}

var defaults = map[string]Config{
	"discord": {Events: []string{"request_submitted", "request_approved", "request_declined", "request_available", "request_failed", "issue_reported", "issue_resolved", "server_problem"}, BotName: "Flixarr", Poster: true},
	"webhook": {Events: []string{"request_available", "request_failed"}, Method: "POST"},
	"push":    {Events: []string{"request_submitted", "request_approved", "request_declined", "request_available", "request_failed", "issue_reported", "issue_resolved", "server_problem", "request_leaving"}},
	"email":   {Events: []string{"request_submitted", "request_approved", "request_declined", "request_available", "request_failed", "issue_reported", "issue_resolved", "server_problem", "request_leaving"}, Port: 587, Security: "starttls", FromName: "Flixarr"},
	"ntfy":    {Events: adminEvents, Server: "https://ntfy.sh"},
}

// adminEvents are the text channels' starting events (they go to one place, like Discord).
var adminEvents = []string{"request_submitted", "request_approved", "request_declined", "request_available", "request_failed", "issue_reported", "issue_resolved", "server_problem"}

func init() {
	for _, ch := range Simple {
		if _, ok := defaults[ch]; !ok {
			defaults[ch] = Config{Events: adminEvents}
		}
	}
}

// Notifier loads the channels and sends.
type Notifier struct {
	Store *store.Store
	Log   *slog.Logger
	HTTP  *http.Client
	mu    sync.Mutex // channel list changes
}

// keyChannels holds every channel (encrypted as a whole: they hold webhook addresses and tokens).
const keyChannels = "notify_channels"

// oldKey is where a service's single channel was kept before channels became a list.
func oldKey(service string) string { return "notify_" + service }

// Defaults are a new channel's starting settings for a service.
func Defaults(service string) Config {
	c := defaults[service]
	c.Events = slices.Clone(c.Events)
	if c.Events == nil {
		c.Events = []string{}
	}
	return c
}

// Channels returns every channel, oldest first, with secrets decrypted.
func (n *Notifier) Channels() []Channel {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.channels()
}

func (n *Notifier) channels() []Channel {
	raw := n.Store.SecretSetting(keyChannels, "")
	if raw == "" {
		return n.migrate()
	}
	out := []Channel{}
	json.Unmarshal([]byte(raw), &out)
	for i := range out {
		if out[i].Events == nil {
			out[i].Events = []string{}
		}
	}
	return out
}

// migrate turns the settings saved before channels became a list into each service's first channel,
// so nothing has to be set up again. The old settings are left where they were.
func (n *Notifier) migrate() []Channel {
	out := []Channel{}
	for _, svc := range Services {
		raw := n.Store.SecretSetting(oldKey(svc), "")
		if raw == "" {
			continue
		}
		c := Defaults(svc)
		json.Unmarshal([]byte(raw), &c)
		if c.Events == nil {
			c.Events = []string{}
		}
		out = append(out, Channel{ID: newID(), Service: svc, Name: Labels[svc], Config: c})
	}
	if err := n.save(out); err != nil && n.Log != nil {
		n.Log.Warn("notification channels not saved", "err", err)
	}
	return out
}

func (n *Notifier) save(list []Channel) error {
	b, _ := json.Marshal(list)
	return n.Store.SetSecretSetting(keyChannels, string(b))
}

func newID() string {
	b := make([]byte, 6)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// Get returns one channel.
func (n *Notifier) Get(id string) (Channel, bool) {
	for _, c := range n.Channels() {
		if c.ID == id {
			return c, true
		}
	}
	return Channel{}, false
}

// Add saves a new channel and returns it with its ID.
func (n *Notifier) Add(c Channel) (Channel, error) {
	if !slices.Contains(Services, c.Service) {
		return c, errors.New("unknown service")
	}
	if err := c.check(); err != nil {
		return c, err
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	list := n.channels()
	if c.Service == "push" && slices.ContainsFunc(list, func(x Channel) bool { return x.Service == "push" }) {
		// Web Push has no settings of its own, so a second one would only send everything twice.
		return c, errors.New("Web Push is already set up; change its events there")
	}
	c.ID = newID()
	return c, n.save(append(list, c))
}

// Update saves a channel's new settings (its service can't change).
func (n *Notifier) Update(c Channel) error {
	if err := c.check(); err != nil {
		return err
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	list := n.channels()
	for i := range list {
		if list[i].ID == c.ID {
			c.Service = list[i].Service
			list[i] = c
			return n.save(list)
		}
	}
	return errors.New("that channel is gone")
}

// Delete removes a channel.
func (n *Notifier) Delete(id string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	list := n.channels()
	out := slices.DeleteFunc(slices.Clone(list), func(c Channel) bool { return c.ID == id })
	if len(out) == len(list) {
		return errors.New("that channel is gone")
	}
	return n.save(out)
}

func (c *Channel) check() error {
	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" {
		c.Name = Labels[c.Service]
	}
	if len(c.Name) > 60 {
		return errors.New("keep the name under 60 characters")
	}
	if c.Events == nil {
		c.Events = []string{}
	}
	c.TestTo, c.TestUser = "", 0
	return c.validate(c.Service)
}

// Service reports whether any channel of a service is on, and every event those channels send.
func (n *Notifier) Service(service string) (on bool, events []string) {
	events = []string{}
	for _, c := range n.Channels() {
		if c.Service != service || !c.On {
			continue
		}
		on = true
		for _, e := range c.Events {
			if !slices.Contains(events, e) {
				events = append(events, e)
			}
		}
	}
	return on, events
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

// PersonalChannels are the channels that reach one person: email and Web Push, and (#124) Discord,
// Telegram, Pushover and ntfy once the person adds their own account there. Webhooks, Slack, Gotify and
// Pushbullet only go to the shared place set up in Settings.
var PersonalChannels = []string{"email", "push", "discord", "telegram", "pushover", "ntfy"}

// AccountServices take a person's own account (Profile → Notifications): a Discord user ID (they're
// mentioned in the Discord message), a Telegram chat ID, a Pushover user key, an ntfy topic. Their own
// news then also goes there, through the channel an admin set up.
var AccountServices = []string{"discord", "telegram", "pushover", "ntfy"}

var accountPattern = map[string]*regexp.Regexp{
	"discord":  regexp.MustCompile(`^[0-9]{15,21}$`),
	"telegram": regexp.MustCompile(`^-?[0-9]{3,20}$`),
	"pushover": regexp.MustCompile(`^[A-Za-z0-9]{30}$`),
	"ntfy":     regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`),
}
var accountWhat = map[string]string{"discord": "Discord user ID (numbers only)", "telegram": "Telegram chat ID (numbers)", "pushover": "Pushover user key (30 letters and numbers)", "ntfy": "ntfy topic (letters, numbers, - and _)"}

func offKey(userID int64) string     { return fmt.Sprintf("notify_off_%d", userID) }
func accountKey(userID int64) string { return fmt.Sprintf("notify_accounts_%d", userID) }

// Off is what a person has switched off: channel → events. Everything is on until they change it.
func (n *Notifier) Off(userID int64) map[string][]string {
	out := map[string][]string{}
	json.Unmarshal([]byte(n.Store.Setting(offKey(userID), "")), &out)
	for _, ch := range PersonalChannels {
		if out[ch] == nil {
			out[ch] = []string{}
		}
	}
	return out
}

// Accounts are a person's own accounts on the AccountServices (kept encrypted; "" = none).
func (n *Notifier) Accounts(userID int64) map[string]string {
	out := map[string]string{}
	json.Unmarshal([]byte(n.Store.SecretSetting(accountKey(userID), "")), &out)
	for _, s := range AccountServices {
		out[s] = strings.TrimSpace(out[s])
	}
	return out
}

// SetAccounts saves a person's own accounts; each is checked against what the service uses.
func (n *Notifier) SetAccounts(userID int64, in map[string]string) error {
	clean := map[string]string{}
	for _, s := range AccountServices {
		v := strings.TrimSpace(in[s])
		if v != "" && !accountPattern[s].MatchString(v) {
			return fmt.Errorf("that doesn't look like a %s", accountWhat[s])
		}
		clean[s] = v
	}
	b, _ := json.Marshal(clean)
	return n.Store.SetSecretSetting(accountKey(userID), string(b))
}

// accountsOn are the own accounts on a service that an event goes to (people who switched it off left out).
func (n *Notifier) accountsOn(m Message, ch string) []string {
	out := []string{}
	for _, u := range n.peopleOn(m, ch) {
		if a := n.Accounts(u.ID)[ch]; a != "" && !slices.Contains(out, a) {
			out = append(out, a)
		}
	}
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
	if slices.Contains(Simple, ch) {
		return c.validateSimple(ch)
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
	"request_leaving":   {"Leaving soon", 0xeab308},
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
		// The requester reads this (email, push): plain words. The full reason stays in m.Error for the
		// shared Discord channel and webhooks.
		m.Text = "Requested by " + user + ". " + requests.PlainError(r)
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

// FromLeaving builds the heads-up for a requested title that Maintainerr removes soon.
func (n *Notifier) FromLeaving(r store.Request, at time.Time) Message {
	m := n.FromRequest("request_leaving", r)
	m.Text = "Leaves Plex on " + at.Local().Format("Monday, January 2") + ". Watch it before then, or ask to keep it in Flixarr."
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

// Send delivers a message to every channel that's on and has the event, in the background. A dev install
// sends only through channels switched to send from it (Test always works).
func (n *Notifier) Send(m Message) {
	for _, c := range n.Channels() {
		if !c.On || !slices.Contains(c.Events, m.Event) || (testinstall.On && !c.FromDev) {
			continue
		}
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			if err := n.deliver(ctx, c.Service, c.Config, m); err != nil {
				n.Log.Warn("notification not sent", "channel", c.Name, "service", c.Service, "event", m.Event, "err", err)
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
		body := discordBody(c, m)
		if m.Event != "test" {
			mention(body, n.accountsOn(m, "discord"))
		}
		return n.post(ctx, http.MethodPost, c.WebhookURL, "", body)
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
	if slices.Contains(Simple, ch) {
		err := n.deliverSimple(ctx, ch, c, m)
		if m.Event == "test" || !slices.Contains(AccountServices, ch) {
			return err
		}
		// A copy to each person's own account on this service (#124), through this channel's token.
		for _, a := range n.accountsOn(m, ch) {
			if a == c.Target {
				continue
			}
			own := c
			own.Target = a
			if e := n.deliverSimple(ctx, ch, own, m); e != nil && err == nil {
				err = e
			}
		}
		return err
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
	if m.Event == "request_failed" && m.Error != "" {
		fields = append(fields, map[string]any{"name": "Details", "value": m.Error})
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

// mention adds the people's Discord accounts to a message, so Discord pings them (only them).
func mention(body map[string]any, ids []string) {
	if len(ids) == 0 {
		return
	}
	tags := []string{}
	for _, id := range ids {
		tags = append(tags, "<@"+id+">")
	}
	content, _ := body["content"].(string)
	body["content"] = strings.TrimSpace(content + " " + strings.Join(tags, " "))
	am, _ := body["allowed_mentions"].(map[string]any)
	if am == nil {
		am = map[string]any{"parse": []string{}}
	}
	am["users"] = ids
	body["allowed_mentions"] = am
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
