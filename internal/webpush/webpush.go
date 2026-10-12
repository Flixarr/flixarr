// Package webpush sends Web Push messages (RFC 8030) with aes128gcm payload encryption (RFC 8291) and
// VAPID sign-in (RFC 8292), using only the standard library.
package webpush

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var b64 = base64.RawURLEncoding

// decode accepts base64url with or without padding (browsers send either).
func decode(s string) ([]byte, error) {
	return b64.DecodeString(strings.TrimRight(s, "="))
}

// Keys is a VAPID key pair: the private scalar and the uncompressed public point, both base64url.
type Keys struct {
	Private string `json:"private"`
	Public  string `json:"public"`
}

// NewKeys makes a VAPID key pair.
func NewKeys() (Keys, error) {
	k, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return Keys{}, err
	}
	return Keys{Private: b64.EncodeToString(k.Bytes()), Public: b64.EncodeToString(k.PublicKey().Bytes())}, nil
}

// Subscription is what the browser's PushManager hands over.
type Subscription struct {
	Endpoint string `json:"endpoint"`
	P256dh   string `json:"p256dh"`
	Auth     string `json:"auth"`
}

func hmacSHA256(key, data []byte) []byte {
	m := hmac.New(sha256.New, key)
	m.Write(data)
	return m.Sum(nil)
}

// encrypt builds an aes128gcm body for the subscription. as and salt are random unless given (tests).
func encrypt(plain []byte, sub Subscription, as *ecdh.PrivateKey, salt []byte) ([]byte, error) {
	uaPub, err := decode(sub.P256dh)
	if err != nil {
		return nil, errors.New("bad subscription key")
	}
	auth, err := decode(sub.Auth)
	if err != nil {
		return nil, errors.New("bad subscription secret")
	}
	ua, err := ecdh.P256().NewPublicKey(uaPub)
	if err != nil {
		return nil, errors.New("bad subscription key")
	}
	if as == nil {
		if as, err = ecdh.P256().GenerateKey(rand.Reader); err != nil {
			return nil, err
		}
	}
	if salt == nil {
		salt = make([]byte, 16)
		rand.Read(salt)
	}
	secret, err := as.ECDH(ua)
	if err != nil {
		return nil, err
	}
	asPub := as.PublicKey().Bytes()
	prkKey := hmacSHA256(auth, secret)
	info := append(append(append([]byte("WebPush: info\x00"), uaPub...), asPub...), 1)
	ikm := hmacSHA256(prkKey, info)
	prk := hmacSHA256(salt, ikm)
	cek := hmacSHA256(prk, []byte("Content-Encoding: aes128gcm\x00\x01"))[:16]
	nonce := hmacSHA256(prk, []byte("Content-Encoding: nonce\x00\x01"))[:12]
	block, err := aes.NewCipher(cek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	sealed := gcm.Seal(nil, nonce, append(append([]byte{}, plain...), 2), nil) // 0x02: last (only) record
	head := make([]byte, 21)
	copy(head, salt)
	binary.BigEndian.PutUint32(head[16:], 4096)
	head[20] = byte(len(asPub))
	return append(append(head, asPub...), sealed...), nil
}

// vapid signs the Authorization header for the push service behind endpoint.
func vapid(keys Keys, endpoint, subject string) (string, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	d, err := decode(keys.Private)
	if err != nil {
		return "", errors.New("bad VAPID key")
	}
	pub, err := decode(keys.Public)
	if err != nil || len(pub) != 65 {
		return "", errors.New("bad VAPID key")
	}
	priv := &ecdsa.PrivateKey{D: new(big.Int).SetBytes(d)}
	priv.PublicKey = ecdsa.PublicKey{Curve: elliptic.P256(), X: new(big.Int).SetBytes(pub[1:33]), Y: new(big.Int).SetBytes(pub[33:])}
	head := b64.EncodeToString([]byte(`{"typ":"JWT","alg":"ES256"}`))
	claims, _ := json.Marshal(map[string]any{"aud": u.Scheme + "://" + u.Host, "exp": time.Now().Add(12 * time.Hour).Unix(), "sub": subject})
	signing := head + "." + b64.EncodeToString(claims)
	sum := sha256.Sum256([]byte(signing))
	r, s, err := ecdsa.Sign(rand.Reader, priv, sum[:])
	if err != nil {
		return "", err
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return "vapid t=" + signing + "." + b64.EncodeToString(sig) + ", k=" + keys.Public, nil
}

// ErrGone means the subscription no longer exists (the browser unsubscribed): forget it.
var ErrGone = errors.New("subscription gone")

// Send pushes a JSON payload to one subscription.
func Send(ctx context.Context, hc *http.Client, keys Keys, subject string, sub Subscription, payload any) error {
	plain, _ := json.Marshal(payload)
	body, err := encrypt(plain, sub, nil, nil)
	if err != nil {
		return err
	}
	auth, err := vapid(keys, sub.Endpoint, subject)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sub.Endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Encoding", "aes128gcm")
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("TTL", "86400")
	req.Header.Set("Urgency", "normal")
	req.Header.Set("Authorization", auth)
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second}
	}
	res, err := hc.Do(req)
	if err != nil {
		return errors.New("couldn't reach the push service")
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound || res.StatusCode == http.StatusGone {
		return ErrGone
	}
	if res.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(res.Body, 200))
		return fmt.Errorf("the push service answered %s %s", res.Status, strings.TrimSpace(string(msg)))
	}
	return nil
}
