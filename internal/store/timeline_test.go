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

func TestFixSteps(t *testing.T) {
	dir := t.TempDir()
	box, _ := secret.Load(dir)
	s, err := Open(dir, box)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	u, _ := s.CreateUser(User{PlexID: 1, Username: "eryn", Role: "Admin"})
	live, _ := s.CreateRequest(Request{Type: "series", TmdbID: 1400, Title: "Jackass", UserID: u.ID, Status: "downloading", Mode: "all"})
	failed, _ := s.CreateRequest(Request{Type: "series", TmdbID: 1400, Title: "Jackass", UserID: u.ID, Status: "failed", Mode: "all"})
	old, _ := s.CreateRequest(Request{Type: "series", TmdbID: 1400, Title: "Jackass", UserID: u.ID, Status: "expired", Mode: "all"})
	last := func(id int64) Event {
		ev, _ := s.RequestEvents(id)
		return ev[len(ev)-1]
	}

	// The whole title: every request still in play gets the step, an ended one doesn't.
	steps := s.AddFixSteps("series", 1400, 0, "Got a different release", u.ID)
	if len(steps) != 2 {
		t.Fatalf("steps: %v", steps)
	}
	if e := last(live.ID); e.Status != "fix" || e.Note != "Got a different release by eryn" {
		t.Fatalf("live: %+v", e)
	}
	if e := last(failed.ID); e.Status != "fix" {
		t.Fatalf("failed: %+v", e)
	}
	if e := last(old.ID); e.Status == "fix" {
		t.Fatalf("ended request got a step: %+v", e)
	}
	// History shows Fix actions from the title events, not once per request.
	h, _ := s.TitleHistory("series", 1400)
	for _, x := range h {
		if x.Kind == "request:fix" {
			t.Fatalf("request fix step in History: %+v", h)
		}
	}

	// One request only; taken back when the action fails.
	steps = s.AddFixSteps("series", 1400, live.ID, "Searched again", u.ID)
	if len(steps) != 1 || last(live.ID).Note != "Searched again by eryn" || last(failed.ID).Note != "Got a different release by eryn" {
		t.Fatalf("one request: %v", steps)
	}
	s.DropFixSteps(steps)
	if last(live.ID).Note != "Got a different release by eryn" {
		t.Fatalf("not taken back: %+v", last(live.ID))
	}
}
