package password

import (
	"encoding/hex"
	"testing"
)

// RFC 7914 section 11: PBKDF2-HMAC-SHA256 test vectors.
func TestPBKDF2Vectors(t *testing.T) {
	for _, c := range []struct {
		pw, salt string
		rounds   int
		want     string
	}{
		{"passwd", "salt", 1, "55ac046e56e3089fec1691c22544b605f94185216dde0465e68b9d57c20dacbc49ca9cccf179b645991664b39d77ef317c71b845b1e30bd509112041d3a19783"},
		{"Password", "NaCl", 80000, "4ddcd8f60b98be21830cee5ef22701f9641a4418d04c0414aeff08876b34ab56a1d425a1225833549adb841b51c9b3176a272bdebba1d078478f62b397f33c8d"},
	} {
		if got := hex.EncodeToString(pbkdf2([]byte(c.pw), []byte(c.salt), c.rounds, 64)); got != c.want {
			t.Errorf("%s/%s: %s", c.pw, c.salt, got)
		}
	}
}

func TestHashCheck(t *testing.T) {
	if _, err := Hash("short"); err == nil {
		t.Fatal("short password accepted")
	}
	h, err := Hash("correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if !Check("correct horse", h) || Check("correct horsE", h) || Check("correct horse", "") || Check("x", "pbkdf2-sha256$0$a$b") {
		t.Fatal("check is wrong")
	}
	if h2, _ := Hash("correct horse"); h2 == h {
		t.Fatal("same salt twice")
	}
}
