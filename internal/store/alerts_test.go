package store

import (
	"testing"
	"time"

	"github.com/Flixarr/flixarr/internal/secret"
)

func TestAlerts(t *testing.T) {
	dir := t.TempDir()
	box, _ := secret.Load(dir)
	st, err := Open(dir, box)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	st.db.Exec(`INSERT INTO users (id, plex_id, username, role, created_at) VALUES (1, 1, 'admin', 'Admin', ?), (2, 2, 'user', 'User', ?)`, now(), now())

	raise := func(detail, sig string) {
		if err := st.RaiseAlert(Alert{Key: "arr:2", Severity: "warning", Title: "Sonarr", Detail: detail, Link: "/settings#services", Sig: sig}); err != nil {
			t.Fatal(err)
		}
	}
	raise("14 downloads need attention", "warning:downloads")
	list, _ := st.ActiveAlerts(1, true)
	if len(list) != 1 || !list[0].Unread || list[0].Detail != "14 downloads need attention" {
		t.Fatalf("first raise: %+v", list)
	}
	if l, _ := st.ActiveAlerts(2, false); len(l) != 0 {
		t.Fatal("non-admin sees an admin alert")
	}
	st.MarkAlertsSeen(1)
	time.Sleep(1100 * time.Millisecond) // timestamps have one-second precision

	raise("13 downloads need attention", "warning:downloads") // same kind of problem: stays read
	if l, _ := st.ActiveAlerts(1, true); len(l) != 1 || l[0].Unread || l[0].Detail != "13 downloads need attention" {
		t.Fatalf("small change: %+v", l)
	}
	raise("13 downloads need attention · 1 health issue", "warning:downloads,health") // new kind: unread again
	if l, _ := st.ActiveAlerts(1, true); !l[0].Unread {
		t.Fatal("new kind of problem should be unread")
	}
	st.ResolveAlert("arr:2")
	if l, _ := st.ActiveAlerts(1, true); len(l) != 0 {
		t.Fatal("resolved alert still active")
	}
	raise("1 download needs attention", "warning:downloads") // comes back: a new alert
	if l, _ := st.ActiveAlerts(1, true); len(l) != 1 || !l[0].Unread {
		t.Fatalf("returning alert: %+v", l)
	}
}
