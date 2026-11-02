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

// A Seerr import writes Requested and Approved in the same second, and "expired" for a request it found
// nowhere: the history keeps them in order, and the import's "expired" reads as imported (#186).
func TestImportedTimelineOrder(t *testing.T) {
	dir := t.TempDir()
	box, _ := secret.Load(dir)
	s, err := Open(dir, box)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	u, _ := s.CreateUser(User{PlexID: 1, Username: "danielle", Role: "User"})
	at := "2026-09-25T19:58:07.000Z"
	id, err := s.ImportRequest(Request{Type: "movie", TmdbID: 1283515, Title: "Verity", UserID: u.ID, Status: "expired", Error: SeerrNotFound, CreatedAt: at, UpdatedAt: at}, 77, u.ID,
		[]Event{{Status: "requested", At: at}, {Status: "approved", Note: "danielle", At: at}, {Status: "expired", Note: SeerrNotFound, At: "2026-10-18T11:24:41Z"}})
	if err != nil {
		t.Fatal(err)
	}
	s.AddTitleEvent("movie", 1283515, "plex", "", 0, "2026-10-27T06:21:25Z")
	h, _ := s.TitleHistory("movie", 1283515)
	got := []string{}
	for _, x := range h {
		got = append(got, x.Kind)
	}
	want := []string{"plex", "request:imported", "request:approved", "request:requested"} // newest first
	if len(got) != len(want) {
		t.Fatalf("history: %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("history: %v, want %v", got, want)
		}
	}
	ev, _ := s.RequestEvents(id)
	if len(ev) != 3 || ev[2].Status != "imported" {
		t.Fatalf("request steps: %+v", ev)
	}
	// A real removal still reads as one.
	r, _ := s.CreateRequest(Request{Type: "movie", TmdbID: 603, Title: "The Matrix", UserID: u.ID, Status: "available"})
	r.Status, r.Error = "expired", "Maintainerr removed it"
	s.UpdateRequest(r)
	if ev, _ := s.RequestEvents(r.ID); ev[len(ev)-1].Status != "expired" {
		t.Fatalf("real removal: %+v", ev)
	}
}
