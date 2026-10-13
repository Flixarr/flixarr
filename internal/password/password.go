// Package password hashes and checks local account passwords: PBKDF2 with HMAC-SHA256 (RFC 8018),
// 600,000 rounds and a random 16-byte salt per password. Stored as "pbkdf2-sha256$rounds$salt$hash".
package password

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
)

// Rounds is the PBKDF2 work factor for new hashes (OWASP's figure for SHA-256).
const Rounds = 600_000

// MinLength is the shortest password accepted.
const MinLength = 8

var b64 = base64.RawStdEncoding

// Hash returns the stored form of a password.
func Hash(pw string) (string, error) {
	if len(pw) < MinLength {
		return "", fmt.Errorf("use at least %d characters", MinLength)
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := pbkdf2([]byte(pw), salt, Rounds, 32)
	return fmt.Sprintf("pbkdf2-sha256$%d$%s$%s", Rounds, b64.EncodeToString(salt), b64.EncodeToString(key)), nil
}

// Check reports whether pw matches a stored hash (in constant time).
func Check(pw, stored string) bool {
	parts := strings.Split(stored, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
		return false
	}
	rounds, err := strconv.Atoi(parts[1])
	salt, err2 := b64.DecodeString(parts[2])
	want, err3 := b64.DecodeString(parts[3])
	if err != nil || err2 != nil || err3 != nil || rounds < 1 || rounds > 10_000_000 || len(want) == 0 {
		return false
	}
	return subtle.ConstantTimeCompare(pbkdf2([]byte(pw), salt, rounds, len(want)), want) == 1
}

// pbkdf2 derives keyLen bytes (RFC 8018 section 5.2) with HMAC-SHA256.
func pbkdf2(pw, salt []byte, rounds, keyLen int) []byte {
	prf := hmac.New(sha256.New, pw)
	out := make([]byte, 0, keyLen)
	var block [4]byte
	for i := uint32(1); len(out) < keyLen; i++ {
		prf.Reset()
		prf.Write(salt)
		binary.BigEndian.PutUint32(block[:], i)
		prf.Write(block[:])
		u := prf.Sum(nil)
		t := append([]byte(nil), u...)
		for n := 1; n < rounds; n++ {
			prf.Reset()
			prf.Write(u)
			u = prf.Sum(u[:0])
			for j := range t {
				t[j] ^= u[j]
			}
		}
		out = append(out, t...)
	}
	return out[:keyLen]
}
