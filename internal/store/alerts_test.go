package store

import (
	"testing"
	"time"

	"github.com/Flixarr/flixarr/internal/secret"
)

// The bell (#93): one alert per thing, red = new since the bell was opened, the number = not tapped,
// tapped news stays a day, to-dos stay until handled.
func TestBell(t *testing.T) {
	dir := t.TempDir()
	box, _ := secret.Load(dir)
	st, err := Open(dir, box)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	st.db.Exec(`INSERT INTO users (id, plex_id, username, role, created_at) VALUES (1, 1, 'admin', 'Admin', ?), (2, 2, 'eryn', 'User', ?)`, now(), now())
	untapped := func(l []Alert) (n int, red bool) {
		for _, a := range l {
			if !a.Tapped {
				n++
			}
			red = red || a.New
		}
		return
	}

	// Two news alerts for Eryn: each its own, both new.
	st.AddNews(Alert{Audience: "user:2", Title: "Ready to watch", Detail: "The Odyssey"})
	st.AddNews(Alert{Audience: "user:2", Title: "Ready to watch", Detail: "The Matrix"})
	l, _ := st.Bell(2, CanNone)
	if n, red := untapped(l); n != 2 || !red || l[0].Detail != "The Matrix" {
		t.Fatalf("two news: %+v", l)
	}
	if l, _ := st.Bell(1, CanAll); len(l) != 0 {
		t.Fatal("an admin sees a user's own alert")
	}
	// Opening the bell: no red, the number stays.
	time.Sleep(1100 * time.Millisecond) // timestamps have one-second precision
	st.OpenedBell(2)
	l, _ = st.Bell(2, CanNone)
	if n, red := untapped(l); n != 2 || red {
		t.Fatalf("opened: %+v", l)
	}
	// Tapping one: off the number, still listed (gray).
	st.TapAlert(2, l[0].ID)
	l, _ = st.Bell(2, CanNone)
	if n, _ := untapped(l); n != 1 || len(l) != 2 {
		t.Fatalf("tapped one: %+v", l)
	}
	// A day later the tapped one drops off; the untapped one waits.
	st.db.Exec(`UPDATE alert_seen SET seen_at = ? WHERE user_id = 2`, time.Now().Add(-25*time.Hour).UTC().Format(time.RFC3339))
	if l, _ := st.Bell(2, CanNone); len(l) != 1 || l[0].Tapped {
		t.Fatalf("a day later: %+v", l)
	}

	// A to-do for people who manage requests: updates quietly, stays when tapped, goes when handled.
	st.RaiseTodo(Alert{Key: "approve:req:7", Audience: "can:manageRequests", Title: "eryn requested", Detail: "Fleabag"})
	st.RaiseTodo(Alert{Key: "approve:req:7", Audience: "can:manageRequests", Title: "eryn requested", Detail: "Fleabag · waiting"})
	l, _ = st.Bell(1, CanAll)
	if len(l) != 1 || l[0].Kind != "todo" || l[0].Detail != "Fleabag · waiting" {
		t.Fatalf("to-do: %+v", l)
	}
	st.TapAlert(1, l[0].ID)
	st.db.Exec(`UPDATE alert_seen SET seen_at = ? WHERE user_id = 1`, time.Now().Add(-48*time.Hour).UTC().Format(time.RFC3339))
	if l, _ := st.Bell(1, CanAll); len(l) != 1 || !l[0].Tapped {
		t.Fatalf("tapped to-do should stay: %+v", l)
	}
	st.ResolveTodo("approve:req:7")
	if l, _ := st.Bell(1, CanAll); len(l) != 0 {
		t.Fatalf("handled to-do still there: %+v", l)
	}
	// A server problem that clears and comes back soon is the same alert; the person who caused a to-do
	// has it tapped already.
	id1, _ := st.RaiseTodo(Alert{Key: "arr:sonarr:1", Audience: "admins", Title: "Sonarr can't be reached"})
	st.ResolveTodo("arr:sonarr:1")
	if id2, _ := st.RaiseTodo(Alert{Key: "arr:sonarr:1", Audience: "admins", Title: "Sonarr can't be reached"}); id2 != id1 {
		t.Fatalf("flapping made a new alert: %d vs %d", id1, id2)
	}
	st.RaiseTodo(Alert{Key: "issue:3", Audience: "can:manageIssues", Title: "New issue"})
	st.TapAlertKey(1, "issue:3")
	for _, a := range func() []Alert { l, _ := st.Bell(1, CanAll); return l }() {
		if a.Key == "issue:3" && !a.Tapped {
			t.Fatal("the reporter's own to-do isn't tapped")
		}
	}
	// News with a key is added once (an update announcement).
	st.AddNews(Alert{Key: "update:0.1.0.300", Audience: "admins", Title: "Flixarr 0.1.0.300 is out"})
	st.AddNews(Alert{Key: "update:0.1.0.300", Audience: "admins", Title: "Flixarr 0.1.0.300 is out"})
	n := 0
	for _, a := range func() []Alert { l, _ := st.Bell(1, CanAll); return l }() {
		if a.Key == "update:0.1.0.300" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("update news %d times", n)
	}
	// Mark all as read taps everything; to-dos stay listed.
	st.TapAll(1, CanAll)
	l, _ = st.Bell(1, CanAll)
	if c, _ := untapped(l); c != 0 || len(l) == 0 {
		t.Fatalf("read all: %+v", l)
	}
}
