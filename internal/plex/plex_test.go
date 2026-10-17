package plex

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func fakeTV(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Plex-Client-Identifier") != "install-1" || r.Header.Get("X-Plex-Product") != "Flixarr" || r.Header.Get("Accept") != "application/json" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v2/pins" && r.URL.Query().Get("strong") == "true":
			w.Write([]byte(`{"id":7,"code":"ABCD","authToken":null}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v2/pins/7":
			w.Write([]byte(`{"id":7,"code":"ABCD","authToken":"tok"}`))
		case r.URL.Path == "/api/v2/user" && r.Header.Get("X-Plex-Token") == "tok":
			w.Write([]byte(`{"id":42,"uuid":"u","username":"someone","email":"s@x.test","thumb":"https://plex.tv/users/u/avatar"}`))
		default:
			w.WriteHeader(http.StatusUnauthorized)
		}
	}))
	old := tvAPI
	tvAPI = srv.URL + "/api/v2"
	t.Cleanup(func() { tvAPI = old; srv.Close() })
	return srv
}

func TestPinSignIn(t *testing.T) {
	fakeTV(t)
	c := New("install-1", "0.1.0.1")
	ctx := context.Background()
	p, err := c.CreatePin(ctx)
	if err != nil || p.ID != 7 || p.Code != "ABCD" || p.AuthToken != "" {
		t.Fatalf("create %+v %v", p, err)
	}
	p, err = c.CheckPin(ctx, 7)
	if err != nil || p.AuthToken != "tok" {
		t.Fatalf("check %+v %v", p, err)
	}
	a, err := c.Me(ctx, p.AuthToken)
	if err != nil || a.ID != 42 || a.Username != "someone" {
		t.Fatalf("me %+v %v", a, err)
	}
	if _, err := c.Me(ctx, "wrong"); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("wrong token: %v", err)
	}
}

func TestAuthURL(t *testing.T) {
	u, err := url.Parse(New("install 1&x", "v").AuthURL("AB CD"))
	if err != nil || u.Host != "app.plex.tv" {
		t.Fatal(u, err)
	}
	q, _ := url.ParseQuery(strings.TrimPrefix(u.EscapedFragment(), "?"))
	if q.Get("clientID") != "install 1&x" || q.Get("code") != "AB CD" || q.Get("context[device][product]") != "Flixarr" {
		t.Fatalf("query %v", q)
	}
}
