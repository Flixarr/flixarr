package store

import (
	"testing"

	"github.com/Flixarr/flixarr/internal/secret"
)

func TestTimelineAndEndedStatus(t *testing.T) {
	dir := t.TempDir()
	box, _ := secret.Load(dir)
	s, err := Open(dir, box)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	u, _ := s.CreateUser(User{PlexID: 1, Username: "eryn", Role: "Admin"})
	r, _ := s.CreateRequest(Request{Type: "movie", TmdbID: 603, Title: "The Matrix", UserID: u.ID, Status: "pending"})
	s.DecideRequest(r.ID, u.ID, "approved")
	r, _ = s.Request(r.ID)
	r.Status = "downloading"
	r.Progress = 10
	s.UpdateRequest(r)
	r.Progress = 60 // progress alone isn't a step
	s.UpdateRequest(r)
	r.Status, r.Error = "failed", "Radarr couldn't find it"
	s.UpdateRequest(r)
	ev, _ := s.RequestEvents(r.ID)
	want := []string{"requested", "approved", "downloading", "failed"}
	if len(ev) != len(want) {
		t.Fatalf("events: %+v", ev)
	}
	for i, w := range want {
		if ev[i].Status != w {
			t.Fatalf("event %d: %+v", i, ev)
		}
	}
	if ev[1].Note != "eryn" || ev[3].Note != "Radarr couldn't find it" {
		t.Fatalf("notes: %+v", ev)
	}
	// Only a failed request: the title's status says so (for Request again). An open one wins over it.
	got, _ := s.RequestStatusFor([]string{"movie:603"})
	if got["movie:603"].Status != "failed" {
		t.Fatalf("ended: %+v", got)
	}
	s.CreateRequest(Request{Type: "movie", TmdbID: 603, Title: "The Matrix", UserID: u.ID, Status: "sent"})
	if got, _ = s.RequestStatusFor([]string{"movie:603"}); got["movie:603"].Status != "sent" {
		t.Fatalf("open over ended: %+v", got)
	}
	// Expired is just requestable again: no status.
	r2, _ := s.CreateRequest(Request{Type: "movie", TmdbID: 949, Title: "Heat", UserID: u.ID, Status: "sent"})
	r2.Status = "expired"
	s.UpdateRequest(r2)
	if got, _ = s.RequestStatusFor([]string{"movie:949"}); len(got) != 0 {
		t.Fatalf("expired shows: %+v", got)
	}
}
