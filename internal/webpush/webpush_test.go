package webpush

import (
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"math/big"
	"strings"
	"testing"
)

// RFC 8291, Appendix A.
func TestRFC8291Vector(t *testing.T) {
	asPriv, _ := decode("yfWPiYE-n46HLnH0KqZOF1fJJU3MYrct3AELtAQ-oRw")
	as, err := ecdh.P256().NewPrivateKey(asPriv)
	if err != nil {
		t.Fatal(err)
	}
	salt, _ := decode("DGv6ra1nlYgDCS1FRnbzlw")
	sub := Subscription{P256dh: "BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV-JvLexhqUzORcxaOzi6-AYWXvTBHm4bjyPjs7Vd8pZGH6SRpkNtoIAiw4", Auth: "BTBZMqHH6r4Tts7J_aSIgg"}
	got, err := encrypt([]byte("When I grow up, I want to be a watermelon"), sub, as, salt)
	if err != nil {
		t.Fatal(err)
	}
	want := "DGv6ra1nlYgDCS1FRnbzlwAAEABBBP4z9KsN6nGRTbVYI_c7VJSPQTBtkgcy27mlmlMoZIIgDll6e3vCYLocInmYWAmS6TlzAC8wEqKK6PBru3jl7A_yl95bQpu6cVPTpK4Mqgkf1CXztLVBSt2Ks3oZwbuwXPXLWyouBWLVWGNWQexSgSxsj_Qulcy4a-fN"
	if b64.EncodeToString(got) != want {
		t.Fatalf("got  %s\nwant %s", b64.EncodeToString(got), want)
	}
}

func TestVAPIDSignature(t *testing.T) {
	k, _ := NewKeys()
	h, err := vapid(k, "https://fcm.googleapis.com/fcm/send/abc", "mailto:x@example.com")
	if err != nil {
		t.Fatal(err)
	}
	tok := strings.TrimPrefix(strings.Split(h, ",")[0], "vapid t=")
	parts := strings.Split(tok, ".")
	sig, _ := decode(parts[2])
	pub, _ := decode(k.Public)
	key := ecdsa.PublicKey{Curve: elliptic.P256(), X: new(big.Int).SetBytes(pub[1:33]), Y: new(big.Int).SetBytes(pub[33:])}
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if !ecdsa.Verify(&key, sum[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])) {
		t.Fatal("VAPID signature doesn't verify")
	}
	claims, _ := decode(parts[1])
	if !strings.Contains(string(claims), `"aud":"https://fcm.googleapis.com"`) {
		t.Fatalf("claims: %s", claims)
	}
}
