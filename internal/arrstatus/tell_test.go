package arrstatus

import (
	"testing"
	"time"

	"github.com/Flixarr/flixarr/internal/store"
)

func TestTellOnlyNewProblems(t *testing.T) {
	sent := 0
	m := &Monitor{OnProblem: func(store.Server, store.Alert) { sent++ }}
	v := store.Server{ID: 1, Name: "Sonarr"}
	m.tell(v, store.Alert{}, "stuck")
	m.tell(v, store.Alert{}, "stuck") // the same problem, next minute
	m.tell(v, store.Alert{}, "stuck,health")
	m.tell(v, store.Alert{}, "health") // one cleared: quieter
	if sent != 2 {
		t.Fatalf("sent %d, want 2", sent)
	}
	// Clears and comes back soon: not again. Back after a long quiet spell: again.
	m.cleared(v)
	m.tell(v, store.Alert{}, "health")
	if sent != 2 {
		t.Fatalf("flapping sent again: %d", sent)
	}
	m.cleared(v)
	m.told[v.ID] = told{sig: "health", cleared: time.Now().Add(-time.Hour)}
	m.tell(v, store.Alert{}, "health")
	if sent != 3 {
		t.Fatalf("back after an hour: %d", sent)
	}
}
