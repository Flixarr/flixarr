package secret

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSealOpen(t *testing.T) {
	dir := t.TempDir()
	b, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(filepath.Join(dir, "secret.key")); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("secret.key = %v, %v", fi, err)
	}
	s := b.Seal("abc123")
	if !Sealed(s) || strings.Contains(s, "abc123") || s == b.Seal("abc123") {
		t.Fatalf("sealed value looks wrong: %q", s)
	}
	if got, err := b.Open(s); err != nil || got != "abc123" {
		t.Fatalf("Open = %q, %v", got, err)
	}
	// Reloading reads the same key file.
	b2, _ := Load(dir)
	if got, err := b2.Open(s); err != nil || got != "abc123" {
		t.Fatalf("reload Open = %q, %v", got, err)
	}
	// Plain values from before encryption pass through.
	if got, _ := b.Open("plainkey"); got != "plainkey" {
		t.Fatalf("plain passthrough = %q", got)
	}
	// A different key can't open it.
	other, _ := Load(t.TempDir())
	if _, err := other.Open(s); err == nil {
		t.Fatal("other key opened the secret")
	}
	if b.Seal("") != "" {
		t.Fatal("empty should stay empty")
	}
}

func TestEnvKey(t *testing.T) {
	t.Setenv("FLIXARR_SECRET_KEY", strings.Repeat("ab", 32))
	dir := t.TempDir()
	if _, err := Load(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "secret.key")); err == nil {
		t.Fatal("secret.key should not be written when the env key is set")
	}
	t.Setenv("FLIXARR_SECRET_KEY", "short")
	if _, err := Load(dir); err == nil {
		t.Fatal("short key accepted")
	}
}
