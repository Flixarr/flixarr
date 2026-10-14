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
	list, _ := st.ActiveAlerts(1, CanAll)
	if len(list) != 1 || !list[0].Unread || list[0].Detail != "14 downloads need attention" {
		t.Fatalf("first raise: %+v", list)
	}
	if l, _ := st.ActiveAlerts(2, CanNone); len(l) != 0 {
		t.Fatal("non-admin sees an admin alert")
	}
	st.MarkAlertsSeen(1)
	time.Sleep(1100 * time.Millisecond) // timestamps have one-second precision

	raise("13 downloads need attention", "warning:downloads") // same kind of problem: stays read
	if l, _ := st.ActiveAlerts(1, CanAll); len(l) != 1 || l[0].Unread || l[0].Detail != "13 downloads need attention" {
		t.Fatalf("small change: %+v", l)
	}
	raise("13 downloads need attention · 1 health issue", "warning:downloads,health") // new kind: unread again
	if l, _ := st.ActiveAlerts(1, CanAll); !l[0].Unread {
		t.Fatal("new kind of problem should be unread")
	}
	st.ResolveAlert("arr:2")
	if l, _ := st.ActiveAlerts(1, CanAll); len(l) != 0 {
		t.Fatal("resolved alert still active")
	}
	raise("1 download needs attention", "warning:downloads") // comes back unread: it was never seen
	if l, _ := st.ActiveAlerts(1, CanAll); len(l) != 1 || !l[0].Unread {
		t.Fatalf("returning alert: %+v", l)
	}
	raise("13 downloads need attention", "warning:downloads,health")
	st.MarkAlertsSeen(1)
	time.Sleep(1100 * time.Millisecond)
	// A problem clearing while another stays: quieter, stays read.
	raise("13 downloads need attention", "warning:downloads")
	if l, _ := st.ActiveAlerts(1, CanAll); l[0].Unread {
		t.Fatal("quieter alert turned unread")
	}
	// Clears and comes back soon after: the same alert, still read.
	st.ResolveAlert("arr:2")
	raise("2 downloads need attention", "warning:downloads")
	if l, _ := st.ActiveAlerts(1, CanAll); len(l) != 1 || l[0].Unread {
		t.Fatalf("flapping alert turned unread: %+v", l)
	}
	// Counts: only a bigger one turns it unread.
	count := func(n string) {
		st.RaiseAlert(Alert{Key: "requests:pending", Severity: "info", Title: n + " waiting", Sig: n})
	}
	count("3")
	st.MarkAlertsSeen(1)
	time.Sleep(1100 * time.Millisecond)
	count("2")
	unread := func() bool {
		l, _ := st.ActiveAlerts(1, CanAll)
		for _, a := range l {
			if a.Key == "requests:pending" {
				return a.Unread
			}
		}
		return false
	}
	if unread() {
		t.Fatal("count going down turned it unread")
	}
	count("3")
	if !unread() {
		t.Fatal("count going up should be unread")
	}
}

func TestAlertsByPermission(t *testing.T) {
	dir := t.TempDir()
	box, _ := secret.Load(dir)
	st, _ := Open(dir, box)
	t.Cleanup(func() { st.Close() })
	st.RaiseAlert(Alert{Key: "requests:pending", Audience: "can:manageRequests", Severity: "info", Title: "1 request waiting", Sig: "1"})
	st.RaiseAlert(Alert{Key: "arr:1", Audience: "admins", Severity: "warning", Title: "Sonarr", Sig: "health"})
	manager := func(p string) bool { return p == "manageRequests" }
	if l, _ := st.ActiveAlerts(5, manager); len(l) != 1 || l[0].Key != "requests:pending" {
		t.Fatalf("a request manager sees the waiting count only: %+v", l)
	}
	if l, _ := st.ActiveAlerts(6, CanNone); len(l) != 0 {
		t.Fatalf("a regular user sees nothing: %+v", l)
	}
	if l, _ := st.ActiveAlerts(1, CanAll); len(l) != 2 {
		t.Fatalf("an admin sees both: %+v", l)
	}
}
