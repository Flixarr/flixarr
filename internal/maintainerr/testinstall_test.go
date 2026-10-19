package maintainerr

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Flixarr/flixarr/internal/testinstall"
)

// A test install reads Maintainerr but never writes to it.
func TestTestInstallReadOnly(t *testing.T) {
	on := testinstall.On
	t.Cleanup(func() { testinstall.On = on })
	testinstall.On = true
	writes := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writes++
		}
		w.Write([]byte(`{"version":"2.0.0"}`))
	}))
	defer srv.Close()
	c := &Client{URL: srv.URL, HTTP: srv.Client()}
	if err := c.Exclude(context.Background(), "123"); err != testinstall.ErrBlocked {
		t.Fatalf("exclude: %v", err)
	}
	if err := c.Postpone(context.Background(), 1, "123", 30); err != testinstall.ErrBlocked {
		t.Fatalf("postpone: %v", err)
	}
	if writes != 0 {
		t.Fatalf("%d writes reached Maintainerr", writes)
	}
}
