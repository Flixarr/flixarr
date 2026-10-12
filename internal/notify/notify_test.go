package notify

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
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
