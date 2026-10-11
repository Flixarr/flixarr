package store

import (
	"testing"

	"github.com/Flixarr/flixarr/internal/secret"
)

func TestServerKeysEncrypted(t *testing.T) {
	dir := t.TempDir()
	box, err := secret.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	st, err := Open(dir, box)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	v, err := st.SaveServer(Server{Kind: "radarr", Name: "Radarr", URL: "http://radarr:7878", APIKey: "topsecretkey", RootFolder: "/movies", QualityProfileID: 1, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if v.APIKey != "topsecretkey" {
		t.Fatalf("saved key read back as %q", v.APIKey)
	}
	var raw string
	st.db.QueryRow(`SELECT api_key FROM servers WHERE id = ?`, v.ID).Scan(&raw)
	if !secret.Sealed(raw) || raw == "topsecretkey" {
		t.Fatalf("key stored as %q", raw)
	}

	// A key stored in plain text (before encryption) is encrypted on the next start.
	st.db.Exec(`UPDATE servers SET api_key = 'oldplainkey' WHERE id = ?`, v.ID)
	st.Close()
	st, err = Open(dir, box)
	if err != nil {
		t.Fatal(err)
	}
	st.db.QueryRow(`SELECT api_key FROM servers WHERE id = ?`, v.ID).Scan(&raw)
	if !secret.Sealed(raw) {
		t.Fatalf("plain key not sealed on start: %q", raw)
	}
	if got, _, _ := st.FirstServer("radarr"); got.APIKey != "oldplainkey" {
		t.Fatalf("key after sealing = %q", got.APIKey)
	}

	// With a different secret key the API key comes back empty instead of failing.
	other, _ := secret.Load(t.TempDir())
	st.Close()
	st, _ = Open(dir, other)
	if got, ok, err := st.FirstServer("radarr"); err != nil || !ok || got.APIKey != "" {
		t.Fatalf("other key: %q %v %v", got.APIKey, ok, err)
	}
}
