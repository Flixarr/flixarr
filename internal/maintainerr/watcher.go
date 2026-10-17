package maintainerr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/Flixarr/flixarr/internal/jobs"
	"github.com/Flixarr/flixarr/internal/store"
)

// Settings keys.
const (
	KeyURL     = "maintainerr_url"
	KeyAPIKey  = "maintainerr_key"     // secret
	KeyProtect = "maintainerr_protect" // days a new request is kept out of the rules after it's available; 0 = off
	keySeen    = "maintainerr_seen"    // the last full read
	keyStats   = "maintainerr_stats"   // collections summary for Settings
)

// DefaultProtectDays keeps a new request out of the rules for a month after it arrives.
const DefaultProtectDays = 30

// HeadsUp is how long before a title leaves its requesters hear about it.
const HeadsUp = 7 * 24 * time.Hour

// Watcher keeps Flixarr's picture of Maintainerr current and acts on it.
type Watcher struct {
	Store *store.Store
	Log   *slog.Logger
	// Leaving is told when a requester should hear that their title leaves soon (once per leave date).
	Leaving func(r store.Request, at time.Time)
	// Changed is told which titles' leaving state changed, so open pages redraw them.
	Changed func(keys []string)

	mu sync.Mutex // one read at a time
}

// Client returns the Maintainerr connection, if one is set up.
func (w *Watcher) Client() (*Client, bool) {
	u := w.Store.Setting(KeyURL, "")
	if u == "" {
		return nil, false
	}
	return &Client{URL: u, APIKey: w.Store.SecretSetting(KeyAPIKey, "")}, true
}

// ProtectDays is how long new requests are kept out of the rules.
func (w *Watcher) ProtectDays() int {
	n, err := strconv.Atoi(w.Store.Setting(KeyProtect, strconv.Itoa(DefaultProtectDays)))
	if err != nil || n < 0 {
		return DefaultProtectDays
	}
	return n
}

// Seen is the stamp of the last full read ("" before the first).
func (w *Watcher) Seen() string { return w.Store.Setting(keySeen, "") }

// Stat summarises a collection for Settings.
type Stat struct {
	ID              int     `json:"id"`
	Title           string  `json:"title"`
	Type            string  `json:"type"`
	Active          bool    `json:"active"`
	DeleteAfterDays int     `json:"deleteAfterDays"`
	Leaving         int     `json:"leaving"`
	Handled         int     `json:"handled"`
	HandledBytes    float64 `json:"handledBytes"`
}

// Stats is the last read's summary.
func (w *Watcher) Stats() []Stat {
	out := []Stat{}
	json.Unmarshal([]byte(w.Store.Setting(keyStats, "")), &out)
	return out
}

func kindOf(collectionType string) (string, bool) {
	switch collectionType {
	case "movie":
		return "movie", true
	case "show":
		return "series", true
	}
	return "", false // season and episode collections remove parts of a show: not shown as the title leaving
}

func key(typ string, id int) string { return fmt.Sprintf("%s:%d", typ, id) }

// Sync reads Maintainerr's collections and then keeps new requests safe, ends fresh keeps that ran out,
// and tells requesters whose titles leave soon. It's the "Maintainerr: read collections" job, and runs
// on Maintainerr's webhook too.
func (w *Watcher) Sync(ctx context.Context) (string, error) {
	c, ok := w.Client()
	if !ok {
		return "Maintainerr isn't connected", nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	cols, err := c.Collections(ctx)
	if err != nil {
		return "", err
	}
	// Titles go on the removal run after their time is up, not the moment it is.
	var runs *jobs.Schedule
	if cron := c.HandlerSchedule(ctx); cron != "" {
		runs, _ = jobs.Parse(cron)
	}
	before, _ := w.Store.LeavingNow(w.Seen())
	seen := time.Now().UTC().Format("2006-01-02T15:04:05.000000000Z")
	items, stats := []store.Leaving{}, []Stat{}
	for _, col := range cols {
		st := Stat{ID: col.ID, Title: col.Title, Type: col.Type, Active: col.Active, DeleteAfterDays: col.DeleteAfterDays, Handled: col.Handled, HandledBytes: col.HandledBytes}
		typ, ok := kindOf(col.Type)
		if ok && col.Active {
			for _, m := range col.Media {
				if m.TmdbID == 0 { // Maintainerr didn't match it: Flixarr may know it by its Plex key
					if it, ok := w.Store.PlexItemByRatingKey(m.PlexID); ok && it.Type == typ {
						m.TmdbID = it.TmdbID
					}
				}
				if m.TmdbID == 0 || m.Added.IsZero() {
					continue
				}
				at := m.Added.Add(time.Duration(col.DeleteAfterDays) * 24 * time.Hour)
				if runs != nil {
					if next := runs.Next(at.In(time.Local)); !next.IsZero() {
						at = next
					}
				}
				items = append(items, store.Leaving{Type: typ, TmdbID: m.TmdbID, PlexID: m.PlexID, CollectionID: col.ID, Collection: col.Title, LeaveAt: at.Format(time.RFC3339)})
				st.Leaving++
			}
		}
		stats = append(stats, st)
	}
	if err := w.Store.SyncLeaving(items, seen); err != nil {
		return "", err
	}
	w.Store.SetSetting(keySeen, seen)
	b, _ := json.Marshal(stats)
	w.Store.SetSetting(keyStats, string(b))

	// Pages redraw the titles that joined, left, or got a new date.
	was := map[string]string{}
	for _, l := range before {
		was[key(l.Type, l.TmdbID)] = l.LeaveAt
	}
	changed := []string{}
	for _, l := range items {
		k := key(l.Type, l.TmdbID)
		if was[k] != l.LeaveAt {
			changed = append(changed, k)
		}
		delete(was, k)
	}
	for k := range was {
		changed = append(changed, k)
	}
	if len(changed) > 0 && w.Changed != nil {
		w.Changed(changed)
	}

	// Asks for titles that aren't leaving anymore (kept, given time, or gone) are done.
	if asks, err := w.Store.KeepAsks(); err == nil && len(asks) > 0 {
		now := map[string]bool{}
		for _, l := range items {
			now[key(l.Type, l.TmdbID)] = true
		}
		for _, a := range asks {
			if !now[key(a.Type, a.TmdbID)] {
				w.Store.ClearKeepAsks(a.Type, a.TmdbID)
			}
		}
		w.asksChanged()
	}
	kept := w.protectFresh(ctx, c, items)
	w.endFreshKeeps(ctx, c)
	w.headsUp(items)
	return fmt.Sprintf("%d titles leaving, %d kept for new requests", len(items), kept), nil
}

// protectFresh keeps titles that were requested and arrived lately out of the rules, so nobody loses a
// title before they've watched it.
func (w *Watcher) protectFresh(ctx context.Context, c *Client, items []store.Leaving) int {
	days := w.ProtectDays()
	if days == 0 {
		return 0
	}
	recent, err := w.Store.AvailableRequestsSince(time.Now().Add(-time.Duration(days) * 24 * time.Hour).UTC().Format(time.RFC3339))
	if err != nil || len(recent) == 0 {
		return 0
	}
	arrived := map[string]time.Time{}
	for _, r := range recent {
		if t, err := time.Parse(time.RFC3339, r.UpdatedAt); err == nil && t.After(arrived[key(r.Type, r.TmdbID)]) {
			arrived[key(r.Type, r.TmdbID)] = t
		}
	}
	n := 0
	for _, l := range items {
		t, ok := arrived[key(l.Type, l.TmdbID)]
		if !ok {
			continue
		}
		if _, has := w.Store.KeepFor(l.Type, l.TmdbID); has {
			continue
		}
		if err := c.Exclude(ctx, l.PlexID); err != nil {
			w.Log.Warn("maintainerr keep", "tmdb", l.TmdbID, "err", err)
			continue
		}
		w.Store.SaveKeep(store.Keep{Type: l.Type, TmdbID: l.TmdbID, PlexID: l.PlexID, Reason: "fresh", UntilAt: t.Add(time.Duration(days) * 24 * time.Hour).Format(time.RFC3339)})
		w.Log.Info("kept a new request out of Maintainerr's rules", "type", l.Type, "tmdb", l.TmdbID)
		n++
	}
	return n
}

// endFreshKeeps lets the rules see titles again once their fresh keep runs out.
func (w *Watcher) endFreshKeeps(ctx context.Context, c *Client) {
	keeps, _ := w.Store.Keeps()
	for _, k := range keeps {
		if k.Reason != "fresh" {
			continue
		}
		if t, err := time.Parse(time.RFC3339, k.UntilAt); err != nil || time.Now().Before(t) {
			continue
		}
		if err := c.Unexclude(ctx, k.PlexID); err != nil {
			w.Log.Warn("maintainerr end keep", "tmdb", k.TmdbID, "err", err)
			continue
		}
		w.Store.DropKeep(k.Type, k.TmdbID)
	}
}

// headsUp tells the people who asked for a title, once per leave date, when it leaves within a week.
func (w *Watcher) headsUp(items []store.Leaving) {
	if w.Leaving == nil {
		return
	}
	for _, l := range items {
		at, err := time.Parse(time.RFC3339, l.LeaveAt)
		if err != nil || time.Until(at) > HeadsUp {
			continue
		}
		if _, kept := w.Store.KeepFor(l.Type, l.TmdbID); kept {
			continue // it's staying
		}
		reqs, _ := w.Store.OpenRequests(l.Type, l.TmdbID)
		for _, r := range reqs {
			if r.Status == "available" && !w.Store.ToldLeaving(r.ID, l.LeaveAt) {
				w.Leaving(r, at)
			}
		}
	}
}

// plexID finds a title's Plex rating key: from the collections, else from Flixarr's Plex list.
func (w *Watcher) plexID(typ string, tmdbID int) (string, error) {
	if l, ok := w.Store.WasLeaving(typ, tmdbID); ok && l.PlexID != "" {
		return l.PlexID, nil
	}
	if got, _ := w.Store.PlexItemsFor([]string{key(typ, tmdbID)}); len(got) > 0 {
		return got[key(typ, tmdbID)].RatingKey, nil
	}
	return "", errors.New("it isn't in Plex")
}

// Keep keeps a title out of Maintainerr's rules for good (an admin's call).
func (w *Watcher) Keep(ctx context.Context, typ string, tmdbID int, by int64) error {
	c, ok := w.Client()
	if !ok {
		return errors.New("Maintainerr isn't connected")
	}
	id, err := w.plexID(typ, tmdbID)
	if err != nil {
		return err
	}
	if err := c.Exclude(ctx, id); err != nil {
		return err
	}
	w.Store.DropKeep(typ, tmdbID) // a fresh keep becomes a lasting one
	w.Store.SaveKeep(store.Keep{Type: typ, TmdbID: tmdbID, PlexID: id, Reason: "kept", ByUser: by})
	w.Store.ClearKeepAsks(typ, tmdbID)
	w.asksChanged()
	go w.Sync(context.WithoutCancel(ctx))
	return nil
}

// LetGo undoes a keep: the rules see the title again.
func (w *Watcher) LetGo(ctx context.Context, typ string, tmdbID int) error {
	c, ok := w.Client()
	if !ok {
		return errors.New("Maintainerr isn't connected")
	}
	id, err := w.plexID(typ, tmdbID)
	if k, has := w.Store.KeepFor(typ, tmdbID); has {
		id, err = k.PlexID, nil
	}
	if err != nil {
		return err
	}
	if err := c.Unexclude(ctx, id); err != nil {
		return err
	}
	w.Store.DropKeep(typ, tmdbID)
	w.Store.ClearKeepAsks(typ, tmdbID)
	w.asksChanged()
	go w.Sync(context.WithoutCancel(ctx))
	return nil
}

// Postpone gives a leaving title more days (counted from today).
func (w *Watcher) Postpone(ctx context.Context, typ string, tmdbID, days int) error {
	c, ok := w.Client()
	if !ok {
		return errors.New("Maintainerr isn't connected")
	}
	l, ok := w.Store.WasLeaving(typ, tmdbID)
	if !ok || l.SeenAt != w.Seen() {
		return errors.New("it isn't leaving")
	}
	if err := c.Postpone(ctx, l.CollectionID, l.PlexID, days); err != nil {
		return err
	}
	w.Store.ClearKeepAsks(typ, tmdbID)
	w.asksChanged()
	_, err := w.Sync(ctx)
	return err
}

// Ask records that someone wants a leaving title kept; the admins' bell counts open asks.
func (w *Watcher) Ask(typ string, tmdbID int, userID int64) error {
	if l, ok := w.Store.WasLeaving(typ, tmdbID); !ok || l.SeenAt != w.Seen() {
		return errors.New("it isn't leaving")
	}
	if err := w.Store.AskToKeep(typ, tmdbID, userID); err != nil {
		return err
	}
	w.asksChanged()
	return nil
}

// asksChanged keeps one to-do per title someone asked to keep (#93) for people who manage requests: who
// asked and when it leaves. It goes once the title is kept, let go or gone.
func (w *Watcher) asksChanged() {
	asks, _ := w.Store.KeepAsks()
	who := map[string][]string{}
	order := []store.KeepAsk{}
	for _, a := range asks {
		k := key(a.Type, a.TmdbID)
		if _, seen := who[k]; !seen {
			order = append(order, a)
		}
		who[k] = append(who[k], a.UserName)
	}
	open := map[string]bool{}
	for _, a := range order {
		k := key(a.Type, a.TmdbID)
		todo := "keep:" + k
		open[todo] = true
		names := who[k]
		title := names[0] + " wants to keep"
		if len(names) == 2 {
			title = names[0] + " and " + names[1] + " want to keep"
		} else if len(names) > 2 {
			title = fmt.Sprintf("%s and %d others want to keep", names[0], len(names)-1)
		}
		detail := k
		if it, _ := w.Store.PlexItemsFor([]string{k}); it[k].Title != "" {
			detail = it[k].Title
		}
		if l, ok := w.Store.WasLeaving(a.Type, a.TmdbID); ok {
			if at, err := time.Parse(time.RFC3339, l.LeaveAt); err == nil {
				detail += " · leaves " + at.Local().Format("Jan 2")
			}
		}
		w.Store.RaiseTodo(store.Alert{Key: todo, Audience: "can:manageRequests", Title: title, Detail: detail, Link: "/leaving"})
		for _, ask := range asks {
			if key(ask.Type, ask.TmdbID) == k {
				w.Store.TapAlertKey(ask.UserID, todo) // they asked; they know
			}
		}
	}
	for _, t := range w.Store.OpenTodoKeys("keep:") {
		if !open[t] {
			w.Store.ResolveTodo(t)
		}
	}
}
