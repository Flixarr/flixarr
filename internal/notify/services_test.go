package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestTextServices(t *testing.T) {
	type hit struct {
		path, auth string
		body       map[string]any
	}
	var mu sync.Mutex
	got := map[string]hit{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var body map[string]any
		json.Unmarshal(b, &body)
		mu.Lock()
		got[strings.SplitN(strings.TrimPrefix(r.URL.Path, "/"), "/", 2)[0]] = hit{r.URL.RequestURI(), r.Header.Get("Authorization"), body}
		mu.Unlock()
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	telegramAPI, pushoverAPI, pushbulletAPI = srv.URL, srv.URL, srv.URL
	n := &Notifier{}
	m := Message{Event: "request_available", Title: "The Matrix (1999)", Text: "Requested by eryn", Link: "https://flixarr.example.com/movie/603"}
	ctx := context.Background()
	for ch, c := range map[string]Config{
		"telegram":   {Token: "123:abc", Target: "-100"},
		"slack":      {Token: srv.URL + "/slack/hook"},
		"pushover":   {Token: "app", Target: "user"},
		"gotify":     {Server: srv.URL + "/gotify", Token: "tok"},
		"ntfy":       {Server: srv.URL + "/ntfy", Target: "flixarr", Token: "tk"},
		"pushbullet": {Token: "pb"},
	} {
		if err := n.deliverSimple(ctx, ch, c, m); err != nil {
			t.Fatalf("%s: %v", ch, err)
		}
	}
	if h := got["bot123:abc"]; h.body["chat_id"] != "-100" || !strings.Contains(h.body["text"].(string), "<b>Now available: The Matrix (1999)</b>") || !strings.Contains(h.body["text"].(string), "Open in Flixarr") {
		t.Fatalf("telegram: %+v", h)
	}
	if h := got["slack"]; !strings.HasPrefix(h.body["text"].(string), "*Now available: The Matrix (1999)*") {
		t.Fatalf("slack: %+v", h)
	}
	if h := got["1"]; h.body["user"] != "user" || h.body["url"] != m.Link {
		t.Fatalf("pushover: %+v", h)
	}
	if h := got["gotify"]; h.path != "/gotify/message?token=tok" || h.body["title"] != "Now available: The Matrix (1999)" {
		t.Fatalf("gotify: %+v", h)
	}
	if h := got["ntfy"]; h.body["topic"] != "flixarr" || h.auth != "Bearer tk" || h.body["click"] != m.Link {
		t.Fatalf("ntfy: %+v", h)
	}
	if h := got["v2"]; h.body["type"] != "link" || !strings.HasPrefix(h.auth, "Basic ") {
		t.Fatalf("pushbullet: %+v", h)
	}
	// Turning one on without its fields says what's missing.
	if err := (Config{On: true, Token: "x"}).validate("telegram"); err == nil || !strings.Contains(err.Error(), "chat ID") {
		t.Fatalf("telegram without a chat: %v", err)
	}
	if err := (Config{On: true, Token: "https://example.com/x"}).validate("slack"); err == nil {
		t.Fatal("a non-Slack URL passed")
	}
}
