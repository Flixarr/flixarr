package server

import (
	"testing"
	"time"
)

func TestSignInPause(t *testing.T) {
	var f failures
	for i := 0; i < maxFails-1; i++ {
		f.add("name:eryn", "ip:1")
	}
	if f.locked("name:eryn") {
		t.Fatal("locked too early")
	}
	f.add("name:eryn", "ip:1")
	if !f.locked("name:eryn") || !f.locked("name:other", "ip:1") {
		t.Fatal("not locked after the limit")
	}
	f.clear("name:eryn")
	if f.locked("name:eryn") {
		t.Fatal("still locked after a good sign-in")
	}
	// Old failures don't count.
	f.seen["name:old"] = []time.Time{time.Now().Add(-time.Hour), time.Now().Add(-time.Hour), time.Now().Add(-time.Hour), time.Now().Add(-time.Hour), time.Now().Add(-time.Hour)}
	if f.locked("name:old") {
		t.Fatal("old failures counted")
	}
}
