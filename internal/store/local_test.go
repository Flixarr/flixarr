package store

import (
	"errors"
	"testing"

	"github.com/Flixarr/flixarr/internal/secret"
)

func TestLocalAccounts(t *testing.T) {
	dir := t.TempDir()
	box, _ := secret.Load(dir)
	s, err := Open(dir, box)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	s.CreateUser(User{PlexID: 1, Username: "owner", Role: "Admin"})
	s.CreateUser(User{PlexID: 2, Username: "grandpa", Role: "User"}) // a Plex account with the same name doesn't clash
	u, err := s.CreateLocalUser("Grandpa", "g@x.test", "User", "hash1")
	if err != nil || !u.Local || u.Owner {
		t.Fatalf("create: %+v %v", u, err)
	}
	if _, err := s.CreateLocalUser("GRANDPA", "", "User", "h"); !errors.Is(err, ErrNameTaken) {
		t.Fatalf("same name, other case: %v", err)
	}
	got, hash, err := s.LocalLogin("grandpa")
	if err != nil || got.ID != u.ID || hash != "hash1" {
		t.Fatalf("login lookup: %+v %q %v", got, hash, err)
	}
	if _, _, err := s.LocalLogin("owner"); !errors.Is(err, ErrNotFound) {
		t.Fatal("a Plex account found as local")
	}
	tok, _ := s.CreateSession(u.ID)
	s.SetPassword(u.ID, "hash2")
	if _, err := s.SessionUser(tok); err == nil {
		t.Fatal("old session survived a password change")
	}
	if s.PasswordHash(u.ID) != "hash2" || s.PasswordHash(1) != "" {
		t.Fatal("hashes")
	}
}
