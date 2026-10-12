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
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Flixarr/flixarr/internal/secret"
	"github.com/Flixarr/flixarr/internal/store"
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

	if err := n.Save("discord", Config{On: true, WebhookURL: "https://example.com/not-discord"}); err == nil {
		t.Fatal("non-Discord URL accepted")
	}
	if err := n.Save("discord", Config{On: true, WebhookURL: srv.URL + "/api/webhooks/1/abc", BotName: "Flixarr", Poster: true, Events: []string{"request_available"}}); err != nil {
		t.Fatal(err)
	}
	if err := n.Save("webhook", Config{On: true, URL: srv.URL + "/hook", Method: "POST", Auth: "Bearer x", Events: []string{"request_available", "request_failed"}}); err != nil {
		t.Fatal(err)
	}
	if raw := st.Setting("notify_discord", ""); strings.Contains(raw, "api/webhooks") {
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
	if err := n.Save("email", Config{On: true, Host: host, Port: p, Security: "none", From: "flixarr@x.test", Events: []string{"request_submitted", "request_available"}}); err != nil {
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
	n.Save("push", Config{On: true, Events: []string{"request_available"}})
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
