// Package secret encrypts small secrets (like Radarr and Sonarr API keys) before they're stored in
// the database, so a copied database file or backup doesn't give them away on its own.
//
// The encryption key comes from FLIXARR_SECRET_KEY when set (keep it outside the data folder for
// the most protection), or from secret.key in the data folder, created on first run. Anyone with
// full access to the running server can still read it; the app needs it to use the secrets.
package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const prefix = "enc:v1:"

// Box seals and opens secrets with one key.
type Box struct{ aead cipher.AEAD }

// Load returns the Box for this server, creating secret.key in dataDir if there's no key yet.
func Load(dataDir string) (*Box, error) {
	if v := strings.TrimSpace(os.Getenv("FLIXARR_SECRET_KEY")); v != "" {
		key, err := decodeKey(v)
		if err != nil {
			return nil, fmt.Errorf("FLIXARR_SECRET_KEY: %w", err)
		}
		return New(key)
	}
	path := filepath.Join(dataDir, "secret.key")
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, []byte(base64.StdEncoding.EncodeToString(key)+"\n"), 0o600); err != nil {
			return nil, err
		}
		return New(key)
	}
	if err != nil {
		return nil, err
	}
	key, err := decodeKey(strings.TrimSpace(string(b)))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return New(key)
}

// decodeKey accepts a 32-byte key as base64 or hex.
func decodeKey(v string) ([]byte, error) {
	if k, err := base64.StdEncoding.DecodeString(v); err == nil && len(k) == 32 {
		return k, nil
	}
	if k, err := hex.DecodeString(v); err == nil && len(k) == 32 {
		return k, nil
	}
	return nil, errors.New("must be 32 bytes, written as base64 or hex")
}

// New returns a Box for a 32-byte key.
func New(key []byte) (*Box, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: aead}, nil
}

// Seal encrypts a secret. An empty secret stays empty.
func (b *Box) Seal(plain string) string {
	if plain == "" {
		return ""
	}
	nonce := make([]byte, b.aead.NonceSize())
	rand.Read(nonce)
	return prefix + base64.RawURLEncoding.EncodeToString(b.aead.Seal(nonce, nonce, []byte(plain), nil))
}

// Sealed reports whether a stored value is already encrypted.
func Sealed(v string) bool { return strings.HasPrefix(v, prefix) }

// Open decrypts a stored secret. Values saved before encryption existed come back as they are.
func (b *Box) Open(stored string) (string, error) {
	if !Sealed(stored) {
		return stored, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(stored, prefix))
	n := b.aead.NonceSize()
	if err != nil || len(raw) < n {
		return "", errors.New("damaged secret")
	}
	plain, err := b.aead.Open(nil, raw[:n], raw[n:], nil)
	if err != nil {
		return "", errors.New("this secret was saved with a different key")
	}
	return string(plain), nil
}
