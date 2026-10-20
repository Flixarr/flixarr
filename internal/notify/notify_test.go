package notify

import (
	"bufio"
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Flixarr/flixarr/internal/secret"
	"github.com/Flixarr/flixarr/internal/store"
	"github.com/Flixarr/flixarr/internal/testinstall"
)

func TestDiscordAndWebhook(t *testing.T) {
	dir := t.TempDir()
	box, _ := secret.Load(dir)
	st, _ := store.Open(dir, box)
	defer st.Close()
	var mu sync.Mutex
	got := map[string]map[string]any{}
	auth := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var m map[string]any
		json.Unmarshal(b, &m)
		mu.Lock()
		got[r.URL.Path] = m
		if r.URL.Path == "/hook" {
			auth = r.Header.Get("Authorization")
		}
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	n := &Notifier{Store: st, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	st.SetSetting("app_url", "https://flixarr.example.com")

	if err := add(n, "discord", Config{On: true, WebhookURL: "https://example.com/not-discord"}); err == nil {
		t.Fatal("non-Discord URL accepted")
	}
	if err := add(n, "discord", Config{On: true, WebhookURL: srv.URL + "/api/webhooks/1/abc", BotName: "Flixarr", Poster: true, Events: []string{"request_available"}}); err != nil {
		t.Fatal(err)
	}
	if err := add(n, "webhook", Config{On: true, URL: srv.URL + "/hook", Method: "POST", Auth: "Bearer x", Events: []string{"request_available", "request_failed"}}); err != nil {
		t.Fatal(err)
	}
	if raw := st.Setting("notify_channels", ""); raw == "" || strings.Contains(raw, "api/webhooks") {
		t.Fatal("Discord webhook stored in plain text")
	}
	r := store.Request{ID: 5, Type: "movie", TmdbID: 603, Title: "The Matrix", Year: "1999", Poster: "/p.jpg", UserName: "eryn", Status: "available"}
	n.Send(n.FromRequest("request_available", r))
	n.Send(n.FromRequest("request_declined", r)) // not chosen anywhere: nothing sent
	time.Sleep(300 * time.Millisecond)
	mu.Lock()
	d := got["/api/webhooks/1/abc"]
	if d == nil {
		t.Fatal("nothing sent to Discord")
	}
	e := d["embeds"].([]any)[0].(map[string]any)
	if e["title"] != "The Matrix (1999)" || e["url"] != "https://flixarr.example.com/movie/603" || e["thumbnail"].(map[string]any)["url"] != "https://image.tmdb.org/t/p/w342/p.jpg" {
		t.Fatalf("discord embed: %v", e)
	}
	h := got["/hook"]
	if h["event"] != "request_available" || h["request"].(map[string]any)["user"] != "eryn" || auth != "Bearer x" {
		t.Fatalf("webhook: %v auth %q", h, auth)
	}
	if len(got) != 2 {
		t.Fatalf("sent %d, want 2", len(got))
	}
	mu.Unlock()
	if err := n.Test(context.Background(), "webhook", Config{URL: srv.URL + "/test", Method: "POST"}); err != nil {
		t.Fatalf("test: %v", err)
	}
}

// fakeSMTP accepts one connection's worth of mail and records the recipients and bodies.
func fakeSMTP(t *testing.T) (addr string, got chan string) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	got = make(chan string, 10)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				r := bufio.NewReader(c)
				fmt.Fprint(c, "220 fake\r\n")
				var rcpt string
				for {
					line, err := r.ReadString('\n')
					if err != nil {
						return
					}
					cmd := strings.ToUpper(strings.TrimSpace(line))
					switch {
					case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
						fmt.Fprint(c, "250 fake\r\n")
					case strings.HasPrefix(cmd, "RCPT"):
						rcpt = strings.TrimSpace(line[strings.Index(line, ":")+1:])
						fmt.Fprint(c, "250 ok\r\n")
					case cmd == "DATA":
						fmt.Fprint(c, "354 go\r\n")
						var body strings.Builder
						for {
							l, _ := r.ReadString('\n')
							if l == ".\r\n" {
								break
							}
							body.WriteString(l)
						}
						got <- rcpt + "|" + body.String()
						fmt.Fprint(c, "250 ok\r\n")
					case cmd == "QUIT":
						fmt.Fprint(c, "221 bye\r\n")
						return
					default:
						fmt.Fprint(c, "250 ok\r\n")
					}
				}
			}(c)
		}
	}()
	t.Cleanup(func() { ln.Close() })
	return ln.Addr().String(), got
}

func TestEmailRecipients(t *testing.T) {
	dir := t.TempDir()
	box, _ := secret.Load(dir)
	st, _ := store.Open(dir, box)
	defer st.Close()
	admin, _ := st.CreateUser(store.User{PlexID: 1, Username: "admin", Email: "admin@x.test", Role: "Admin"})
	eryn, _ := st.CreateUser(store.User{PlexID: 2, Username: "eryn", Email: "eryn@x.test", Role: "User"})
	_ = admin
	addr, got := fakeSMTP(t)
	host, port, _ := net.SplitHostPort(addr)
	p, _ := strconv.Atoi(port)
	n := &Notifier{Store: st, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	if err := add(n, "email", Config{On: true, Host: host, Port: p, Security: "none", From: "flixarr@x.test", Events: []string{"request_submitted", "request_available"}}); err != nil {
		t.Fatal(err)
	}
	r := store.Request{ID: 1, Type: "movie", TmdbID: 603, Title: "The Matrix", UserID: eryn.ID, UserName: "eryn", Status: "pending"}
	n.Send(n.FromRequest("request_submitted", r)) // to the admin
	n.Send(n.FromRequest("request_available", r)) // to eryn
	seen := map[string]string{}
	for i := 0; i < 2; i++ {
		select {
		case m := <-got:
			to, body, _ := strings.Cut(m, "|")
			seen[to] = body
		case <-time.After(3 * time.Second):
			t.Fatalf("only %d mails: %v", i, seen)
		}
	}
	if !strings.Contains(seen["<admin@x.test>"], "New request") || !strings.Contains(seen["<eryn@x.test>"], "Now available") {
		t.Fatalf("mails: %v", seen)
	}
}

func TestPushRoutingAndGone(t *testing.T) {
	dir := t.TempDir()
	box, _ := secret.Load(dir)
	st, _ := store.Open(dir, box)
	defer st.Close()
	st.CreateUser(store.User{PlexID: 1, Username: "admin", Role: "Admin"})
	eryn, _ := st.CreateUser(store.User{PlexID: 2, Username: "eryn", Role: "User"})
	var mu sync.Mutex
	hits := map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits[r.URL.Path] = r.Header.Get("Authorization") + "|" + r.Header.Get("Content-Encoding")
		mu.Unlock()
		if r.URL.Path == "/gone" {
			w.WriteHeader(http.StatusGone)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()
	ua, _ := ecdh.P256().GenerateKey(rand.Reader)
	key := base64.RawURLEncoding.EncodeToString(ua.PublicKey().Bytes())
	st.SavePushSub(store.PushSub{Endpoint: srv.URL + "/eryn", UserID: eryn.ID, P256dh: key, Auth: "BTBZMqHH6r4Tts7J_aSIgg"})
	st.SavePushSub(store.PushSub{Endpoint: srv.URL + "/gone", UserID: eryn.ID, P256dh: key, Auth: "BTBZMqHH6r4Tts7J_aSIgg"})
	n := &Notifier{Store: st, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	add(n, "push", Config{On: true, Events: []string{"request_available"}})
	n.Send(n.FromRequest("request_available", store.Request{ID: 1, Type: "movie", TmdbID: 603, Title: "The Matrix", UserID: eryn.ID}))
	time.Sleep(500 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if !strings.HasPrefix(hits["/eryn"], "vapid t=") || !strings.HasSuffix(hits["/eryn"], "|aes128gcm") {
		t.Fatalf("push: %v", hits)
	}
	if subs, _ := st.PushSubs(eryn.ID); len(subs) != 1 {
		t.Fatalf("gone subscription kept: %d", len(subs))
	}
}

func TestPersonalOff(t *testing.T) {
	dir := t.TempDir()
	box, _ := secret.Load(dir)
	st, _ := store.Open(dir, box)
	t.Cleanup(func() { st.Close() })
	n := &Notifier{Store: st}
	n.SetOff(7, map[string][]string{"push": {"request_available", "nonsense"}, "slack": {"request_failed"}})
	off := n.Off(7)
	if len(off["push"]) != 1 || off["push"][0] != "request_available" || len(off["email"]) != 0 || off["slack"] != nil {
		t.Fatalf("off: %v", off)
	}
	user := &store.User{Perms: []string{"requestMovies"}}
	admin := &store.User{Perms: []string{"admin"}}
	if ev := EventsFor(user); slices.Contains(ev, "request_submitted") || slices.Contains(ev, "server_problem") || !slices.Contains(ev, "request_available") {
		t.Fatalf("user events: %v", ev)
	}
	if ev := EventsFor(admin); !slices.Contains(ev, "server_problem") {
		t.Fatalf("admin events: %v", ev)
	}
}

// add saves a new channel of a service (the tests' shorthand).
func add(n *Notifier, service string, c Config) error {
	_, err := n.Add(Channel{Service: service, Config: c})
	return err
}

// Settings saved before channels became a list turn into each service's first channel.
func TestMigrateOldSettings(t *testing.T) {
	dir := t.TempDir()
	box, _ := secret.Load(dir)
	st, _ := store.Open(dir, box)
	defer st.Close()
	st.SetSecretSetting("notify_discord", `{"on":true,"events":["request_available"],"webhookUrl":"https://discord.com/api/webhooks/1/a","botName":"Bot"}`)
	st.SetSecretSetting("notify_email", `{"on":false,"events":[],"host":"smtp.x.test","port":25,"security":"none","from":"a@x.test"}`)
	n := &Notifier{Store: st}
	list := n.Channels()
	if len(list) != 2 || list[0].Service != "discord" || list[0].Name != "Discord" || !list[0].On || list[0].BotName != "Bot" ||
		list[0].WebhookURL != "https://discord.com/api/webhooks/1/a" || list[1].Service != "email" || list[1].Host != "smtp.x.test" {
		t.Fatalf("migrated: %+v", list)
	}
	if st.SecretSetting("notify_discord", "") == "" {
		t.Fatal("old setting removed")
	}
	if again := n.Channels(); again[0].ID != list[0].ID {
		t.Fatal("migrated twice")
	}
	if on, ev := n.Service("discord"); !on || len(ev) != 1 {
		t.Fatalf("service: %v %v", on, ev)
	}
	if on, _ := n.Service("email"); on {
		t.Fatal("email off but reported on")
	}
}

// Two channels of one service each send only their own events; update, delete and the one Web Push.
func TestSeveralChannelsPerService(t *testing.T) {
	dir := t.TempDir()
	box, _ := secret.Load(dir)
	st, _ := store.Open(dir, box)
	defer st.Close()
	var mu sync.Mutex
	got := map[string][]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var m map[string]any
		json.NewDecoder(r.Body).Decode(&m)
		mu.Lock()
		got[r.URL.Path] = append(got[r.URL.Path], fmt.Sprint(m["event"]))
		mu.Unlock()
	}))
	defer srv.Close()
	n := &Notifier{Store: st, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	issues, err := n.Add(Channel{Service: "webhook", Name: "  Issues  ", Config: Config{On: true, URL: srv.URL + "/issues", Method: "POST", Events: []string{"issue_reported"}}})
	if err != nil || issues.Name != "Issues" || issues.ID == "" {
		t.Fatalf("add: %+v %v", issues, err)
	}
	reqs, _ := n.Add(Channel{Service: "webhook", Config: Config{On: true, URL: srv.URL + "/requests", Method: "POST", Events: []string{"request_available"}}})
	if reqs.Name != "Webhook" {
		t.Fatalf("default name %q", reqs.Name)
	}
	r := store.Request{ID: 1, Type: "movie", TmdbID: 603, Title: "The Matrix"}
	n.Send(n.FromRequest("request_available", r))
	n.Send(n.FromIssue("issue_reported", store.Issue{ID: 2, Type: "movie", TmdbID: 603, Title: "The Matrix", Kind: "Video"}))
	time.Sleep(300 * time.Millisecond)
	mu.Lock()
	if fmt.Sprint(got["/issues"]) != "[issue_reported]" || fmt.Sprint(got["/requests"]) != "[request_available]" {
		t.Fatalf("sent: %v", got)
	}
	mu.Unlock()

	reqs.On = false
	reqs.Service = "discord" // ignored: a channel's service can't change
	if err := n.Update(reqs); err != nil {
		t.Fatal(err)
	}
	if c, _ := n.Get(reqs.ID); c.On || c.Service != "webhook" {
		t.Fatalf("updated: %+v", c)
	}
	if err := n.Delete(issues.ID); err != nil || len(n.Channels()) != 1 {
		t.Fatalf("delete: %v, %d left", err, len(n.Channels()))
	}
	if err := n.Delete(issues.ID); err == nil {
		t.Fatal("deleted twice")
	}
	if _, err := n.Add(Channel{Service: "push", Config: Config{On: true}}); err != nil {
		t.Fatal(err)
	}
	if _, err := n.Add(Channel{Service: "push"}); err == nil {
		t.Fatal("second Web Push accepted")
	}
	if _, err := n.Add(Channel{Service: "carrier-pigeon"}); err == nil {
		t.Fatal("unknown service accepted")
	}
}

// A test install sends no automatic notifications, but Send test still works.
func TestTestInstallQuiet(t *testing.T) {
	on := testinstall.On
	t.Cleanup(func() { testinstall.On = on })
	testinstall.On = true
	dir := t.TempDir()
	box, _ := secret.Load(dir)
	st, _ := store.Open(dir, box)
	defer st.Close()
	var mu sync.Mutex
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { mu.Lock(); hits++; mu.Unlock() }))
	defer srv.Close()
	n := &Notifier{Store: st, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	add(n, "webhook", Config{On: true, URL: srv.URL, Method: "POST", Events: []string{"request_available"}})
	n.Send(n.FromRequest("request_available", store.Request{ID: 1, Type: "movie", TmdbID: 603, Title: "The Matrix"}))
	time.Sleep(200 * time.Millisecond)
	mu.Lock()
	if hits != 0 {
		t.Fatalf("sent %d automatic notifications", hits)
	}
	mu.Unlock()
	if err := n.Test(context.Background(), "webhook", Config{URL: srv.URL, Method: "POST"}); err != nil || hits != 1 {
		t.Fatalf("send test: %v, %d sent", err, hits)
	}
}

// Own accounts (#124): checked when saved, and news about someone's request also goes to their own
// account through the channel (a Telegram copy to their chat, a Discord mention).
func TestPersonalAccounts(t *testing.T) {
	dir := t.TempDir()
	box, _ := secret.Load(dir)
	st, _ := store.Open(dir, box)
	t.Cleanup(func() { st.Close() })
	n := &Notifier{Store: st, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	u, _ := st.CreateUser(store.User{PlexID: 2, Username: "eryn", Role: "User"})
	if err := n.SetAccounts(u.ID, map[string]string{"discord": "not a number"}); err == nil {
		t.Fatal("bad Discord ID saved")
	}
	if err := n.SetAccounts(u.ID, map[string]string{"discord": "123456789012345678", "telegram": "555123"}); err != nil {
		t.Fatal(err)
	}
	m := Message{Event: "request_available", UserID: u.ID, Title: "Heat"}
	if got := n.accountsOn(m, "telegram"); len(got) != 1 || got[0] != "555123" {
		t.Fatalf("telegram: %v", got)
	}
	n.SetOff(u.ID, map[string][]string{"telegram": {"request_available"}})
	if got := n.accountsOn(m, "telegram"); len(got) != 0 {
		t.Fatalf("switched off but still sent: %v", got)
	}
	body := discordBody(Config{}, m)
	mention(body, n.accountsOn(m, "discord"))
	if body["content"] != "<@123456789012345678>" {
		t.Fatalf("mention: %v", body)
	}
	if am := body["allowed_mentions"].(map[string]any); am["users"].([]string)[0] != "123456789012345678" {
		t.Fatalf("allowed mentions: %v", am)
	}
}
