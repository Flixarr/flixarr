package jobs

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/Flixarr/flixarr/internal/secret"
	"github.com/Flixarr/flixarr/internal/store"
)

func TestJobEvents(t *testing.T) {
	dir := t.TempDir()
	box, _ := secret.Load(dir)
	st, err := store.Open(dir, box)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	r := &Runner{Store: st, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), running: map[string]bool{}}
	ch, stop := r.Subscribe()
	defer stop()
	j := &Job{ID: "x", Name: "X", Run: func(context.Context) (string, error) { return "ok", errors.New("boom") }}
	r.mu.Lock()
	r.jobs = append(r.jobs, j)
	r.mu.Unlock()
	go r.exec(context.Background(), j)
	got := []Event{}
	timeout := time.After(2 * time.Second)
	for len(got) < 2 {
		select {
		case e := <-ch:
			got = append(got, e)
		case <-timeout:
			t.Fatalf("only got %v", got)
		}
	}
	if got[0].Phase != "start" || got[1].Phase != "done" || got[1].Error != "boom" || got[1].Result != "ok" {
		t.Fatalf("events = %+v", got)
	}
}
