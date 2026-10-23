// Package live carries changes to open browsers as they happen (request status, bell alerts, new titles
// in Plex, job runs), so pages don't have to keep asking. Events are numbered and the last few hundred
// are kept: a browser that reconnects with Last-Event-ID gets what it missed, or "resync" (reload
// everything once) when it was gone too long or the server restarted.
package live

import (
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Flixarr/flixarr/internal/store"
)

// Event is one change. For decides who receives it; nil = everyone signed in.
type Event struct {
	ID   string                   `json:"-"`
	Type string                   `json:"type"` // request | alerts | plex | job | resync
	Data any                      `json:"data,omitempty"`
	For  func(u *store.User) bool `json:"-"`
	seq  uint64
}

// Hub fans events out to subscribers.
type Hub struct {
	mu     sync.Mutex
	boot   string
	seq    uint64
	recent []Event // ring of the newest events, for reconnects
	subs   map[chan Event]struct{}
}

const keep = 500

// New returns a hub. Event IDs carry the boot time, so IDs from before a restart are recognised.
func New() *Hub {
	return &Hub{boot: strconv.FormatInt(time.Now().UnixNano(), 36), subs: map[chan Event]struct{}{}}
}

// Publish sends an event to every subscriber (slow ones skip it rather than hold everyone up).
func (h *Hub) Publish(e Event) {
	h.mu.Lock()
	h.seq++
	e.seq = h.seq
	e.ID = h.boot + "-" + strconv.FormatUint(h.seq, 10)
	h.recent = append(h.recent, e)
	if len(h.recent) > keep {
		h.recent = h.recent[len(h.recent)-keep:]
	}
	subs := make([]chan Event, 0, len(h.subs))
	for c := range h.subs {
		subs = append(subs, c)
	}
	h.mu.Unlock()
	for _, c := range subs {
		select {
		case c <- e:
		default:
		}
	}
}

// Subscribe starts receiving events. lastID is the browser's Last-Event-ID: the events after it are
// returned to send first, and resync=true means they can't be (gone too long, or another boot).
func (h *Hub) Subscribe(lastID string) (ch chan Event, missed []Event, resync bool, stop func()) {
	ch = make(chan Event, 64)
	h.mu.Lock()
	defer h.mu.Unlock()
	h.subs[ch] = struct{}{}
	stop = func() {
		h.mu.Lock()
		delete(h.subs, ch)
		h.mu.Unlock()
	}
	if lastID == "" {
		return ch, nil, false, stop // a fresh page loads its own data
	}
	boot, n, ok := strings.Cut(lastID, "-")
	seq, err := strconv.ParseUint(n, 10, 64)
	if !ok || err != nil || boot != h.boot {
		return ch, nil, true, stop
	}
	if len(h.recent) > 0 && seq < h.recent[0].seq-1 {
		return ch, nil, true, stop // older than what's kept
	}
	for _, e := range h.recent {
		if e.seq > seq {
			missed = append(missed, e)
		}
	}
	return ch, missed, false, stop
}

// Audience helpers.

// Admins: people with the admin switch.
func Admins(u *store.User) bool { return u.Can("admin") }

// ForAudience turns an alert audience ("everyone", "admins", "user:<id>", "can:<permission>") into a filter.
func ForAudience(a string) func(u *store.User) bool {
	if a == "everyone" {
		return nil
	}
	return func(u *store.User) bool { return store.SeenBy(a, u.ID, u.Can) }
}
