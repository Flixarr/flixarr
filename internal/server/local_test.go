package server

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Flixarr/flixarr/internal/secret"
	"github.com/Flixarr/flixarr/internal/store"
)

func TestSignInPause(t *testing.T) {
	var f failures
	for i := 0; i < maxFails-1; i++ {
		f.add("name:eryn", "ip:1")
	}
	if f.locked("name:eryn") {
		t.Fatal("locked too early")
	}
	f.add("name:eryn", "ip:1")
	if !f.locked("name:eryn") || !f.locked("name:other", "ip:1") {
		t.Fatal("not locked after the limit")
	}
	f.clear("name:eryn")
	if f.locked("name:eryn") {
		t.Fatal("still locked after a good sign-in")
	}
	// Old failures don't count.
	f.seen["name:old"] = []time.Time{time.Now().Add(-time.Hour), time.Now().Add(-time.Hour), time.Now().Add(-time.Hour), time.Now().Add(-time.Hour), time.Now().Add(-time.Hour)}
	if f.locked("name:old") {
		t.Fatal("old failures counted")
	}
}

func TestClientIPTrustsOnlyLocalProxies(t *testing.T) {
	r := httptest.NewRequest("POST", "/", nil)
	r.Header.Set("X-Forwarded-For", "1.2.3.4")
	r.RemoteAddr = "203.0.113.9:5000" // straight from the internet: the header is ignored
	if got := clientIP(r); got != "203.0.113.9" {
		t.Fatalf("public: %s", got)
	}
	r.RemoteAddr = "172.18.0.2:5000" // a reverse proxy container
	if got := clientIP(r); got != "1.2.3.4" {
		t.Fatalf("proxy: %s", got)
	}
}

func TestSignInMode(t *testing.T) {
	dir := t.TempDir()
	box, _ := secret.Load(dir)
	st, _ := store.Open(dir, box)
	t.Cleanup(func() { st.Close() })
	s := &Server{Store: st}
	if s.signIn() != "both" {
		t.Fatalf("default: %s", s.signIn())
	}
	st.SetSetting(KeySignIn, "plex")
	w := httptest.NewRecorder()
	s.localLogin(w, httptest.NewRequest("POST", "/api/auth/local", strings.NewReader(`{"username":"grandpa","password":"x"}`)))
	if w.Code != 403 {
		t.Fatalf("Plex only, a username sign-in: %d", w.Code)
	}
	st.SetSetting(KeySignIn, "nonsense")
	if s.signIn() != "both" {
		t.Fatal("an unknown value should mean both")
	}
}
