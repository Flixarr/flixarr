package live

import "testing"

func TestReplayAndResync(t *testing.T) {
	h := New()
	h.Publish(Event{Type: "a"})
	ch, _, _, stop := h.Subscribe("")
	defer stop()
	h.Publish(Event{Type: "b"})
	first := <-ch
	h.Publish(Event{Type: "c"})
	<-ch
	// Reconnect after b: gets c only.
	_, missed, resync, stop2 := h.Subscribe(first.ID)
	defer stop2()
	if resync || len(missed) != 1 || missed[0].Type != "c" {
		t.Fatalf("missed %v resync %v", missed, resync)
	}
	// An ID from another boot: resync.
	if _, _, resync, s := h.Subscribe("zzz-3"); !resync {
		t.Fatal("other boot should resync")
	} else {
		s()
	}
	// Older than what's kept: resync.
	for i := 0; i < keep+5; i++ {
		h.Publish(Event{Type: "x"})
	}
	if _, _, resync, s := h.Subscribe(first.ID); !resync {
		t.Fatal("too old should resync")
	} else {
		s()
	}
}
