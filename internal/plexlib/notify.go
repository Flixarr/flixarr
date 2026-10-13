package plexlib

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Plex Media Server tells connected apps when its libraries change (/:/websockets/notifications; no
// Plex Pass needed). Listen keeps that connection open and, when something is added, reads the newest
// additions straight away: titles show as available within seconds instead of on the next scan. When
// something is deleted, it's dropped at once; and after any library scan (which is when Plex notices files
// that are gone) a full read catches whatever the delete messages missed.

// Live reports whether Plex's change messages are coming in right now.
func (l *Library) Live() bool { return l.live.Load() }

// Listen runs until ctx ends, reconnecting with a growing pause when the connection drops. New titles
// it reads go to OnAdded like any scan's.
func (l *Library) Listen(ctx context.Context) {
	wait := 5 * time.Second
	for ctx.Err() == nil {
		base, tok, ok := l.conn()
		if !ok {
			sleep(ctx, time.Minute) // no Plex server yet
			continue
		}
		start := time.Now()
		err := l.listenOnce(ctx, base, tok)
		l.live.Store(false)
		if ctx.Err() != nil {
			return
		}
		if time.Since(start) > time.Minute {
			wait = 5 * time.Second // it worked for a while: retry quickly
		}
		l.Log.Info("plex live messages stopped", "err", err, "retry in", wait)
		sleep(ctx, wait)
		wait = min(wait*2, 5*time.Minute)
	}
}

func sleep(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}

func (l *Library) listenOnce(ctx context.Context, base, token string) error {
	u, err := url.Parse(strings.TrimRight(base, "/") + "/:/websockets/notifications")
	if err != nil {
		return err
	}
	q := u.Query()
	q.Set("X-Plex-Token", token)
	u.RawQuery = q.Encode()
	conn, r, err := dialWS(ctx, u)
	if err != nil {
		return err
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	l.live.Store(true)
	l.Log.Info("plex live messages connected")

	var pending atomic.Bool // a read of the newest additions is already scheduled
	added := func() {
		if pending.Swap(true) {
			return
		}
		time.AfterFunc(4*time.Second, func() { // Plex sends several messages per title; read once
			pending.Store(false)
			l.RecentScan(ctx)
		})
	}
	var gone struct {
		sync.Mutex
		ids      []string
		episodes bool
		timer    *time.Timer
	}
	removed := func(ids []string, episodes bool) {
		gone.Lock()
		defer gone.Unlock()
		gone.ids = append(gone.ids, ids...)
		gone.episodes = gone.episodes || episodes
		if gone.timer != nil {
			return
		}
		gone.timer = time.AfterFunc(4*time.Second, func() { // a show's delete sends one message per episode
			gone.Lock()
			ids, episodes := gone.ids, gone.episodes
			gone.ids, gone.episodes, gone.timer = nil, false, nil
			gone.Unlock()
			l.dropDeleted(ctx, base, token, ids, episodes)
		})
	}
	// Messages may have been missed while disconnected: read everything once now.
	l.reconcile(ctx)
	for {
		conn.SetReadDeadline(time.Now().Add(3 * time.Minute)) // Plex pings well within this
		op, msg, err := readFrame(r, conn)
		if err != nil {
			return err
		}
		switch op {
		case 0x9: // ping
			writeFrame(conn, 0xA, msg)
		case 0x8: // close
			return errors.New("closed by Plex")
		case 0x1:
			if libraryAdded(msg) {
				added()
			}
			if scanEnded(msg) {
				l.reconcile(ctx)
			}
			if ids, episodes := libraryRemoved(msg); len(ids) > 0 || episodes {
				removed(ids, episodes)
			}
		}
	}
}

// fullEvery is the least time between full reads caused by Plex's own scans; one asked for sooner runs
// when the gap is up, so the last change is never left out.
const fullEvery = 10 * time.Minute

// reconcile runs a full read of the libraries now, or once fullEvery has passed since the last one.
func (l *Library) reconcile(ctx context.Context) {
	if l.fullDue.Swap(true) {
		return // one is already waiting
	}
	wait := time.Until(time.Unix(l.lastFull.Load(), 0).Add(fullEvery))
	time.AfterFunc(max(wait, 5*time.Second), func() {
		l.fullDue.Store(false)
		if ctx.Err() == nil {
			if _, err := l.FullScan(ctx); err != nil {
				l.Log.Warn("plex full read", "err", err)
			}
		}
	})
}

// dropDeleted forgets the movies and shows Plex says it deleted (checked first: a title that's still there
// stays) and passes on that episodes of some show went.
func (l *Library) dropDeleted(ctx context.Context, base, token string, ids []string, episodes bool) {
	gone := []string{}
	for _, id := range ids {
		var out struct{}
		if err := l.get(ctx, base, "/library/metadata/"+url.PathEscape(id), token, &out); errors.Is(err, errNotFound) {
			gone = append(gone, id)
		}
	}
	keys, err := l.Store.RemovePlexItems(gone)
	if err != nil {
		l.Log.Warn("plex removed", "err", err)
	}
	if (len(keys) > 0 || episodes) && l.OnRemoved != nil {
		l.OnRemoved(keys, episodes)
	}
}

// libraryRemoved reads a timeline message for deletes (state 9): the IDs of deleted movies and shows,
// and whether seasons or episodes went.
func libraryRemoved(msg []byte) (ids []string, episodes bool) {
	var m struct {
		NotificationContainer struct {
			Type          string `json:"type"`
			TimelineEntry []struct {
				Type   int             `json:"type"`
				State  int             `json:"state"`
				ItemID json.RawMessage `json:"itemID"` // Plex sends it as a string ("75119"); a number is accepted too
			} `json:"TimelineEntry"`
		} `json:"NotificationContainer"`
	}
	if json.Unmarshal(msg, &m) != nil || m.NotificationContainer.Type != "timeline" {
		return nil, false
	}
	for _, e := range m.NotificationContainer.TimelineEntry {
		if e.State != 9 {
			continue
		}
		switch e.Type {
		case 1, 2:
			if id := strings.Trim(string(e.ItemID), `"`); id != "" {
				ids = append(ids, id)
			}
		case 3, 4:
			episodes = true
		}
	}
	return ids, episodes
}

// scanEnded: Plex finished scanning a library, which is when it notices files that are gone.
func scanEnded(msg []byte) bool {
	var m struct {
		NotificationContainer struct {
			Type                 string `json:"type"`
			ActivityNotification []struct {
				Event    string `json:"event"`
				Activity struct {
					Type string `json:"type"`
				} `json:"Activity"`
			} `json:"ActivityNotification"`
		} `json:"NotificationContainer"`
	}
	if json.Unmarshal(msg, &m) != nil || m.NotificationContainer.Type != "activity" {
		return false
	}
	for _, a := range m.NotificationContainer.ActivityNotification {
		if a.Event == "ended" && a.Activity.Type == "library.update.section" {
			return true
		}
	}
	return false
}

// libraryAdded: a library scan finished (activity "library.update.section" ended), or a timeline message
// about a movie, show, season or episode finishing processing. Either can mean new titles.
func libraryAdded(msg []byte) bool {
	var m struct {
		NotificationContainer struct {
			Type          string `json:"type"`
			TimelineEntry []struct {
				Type  int `json:"type"`  // 1 movie, 2 show, 3 season, 4 episode
				State int `json:"state"` // 5 = done
			} `json:"TimelineEntry"`
			ActivityNotification []struct {
				Event    string `json:"event"`
				Activity struct {
					Type string `json:"type"`
				} `json:"Activity"`
			} `json:"ActivityNotification"`
		} `json:"NotificationContainer"`
	}
	if json.Unmarshal(msg, &m) != nil {
		return false
	}
	switch m.NotificationContainer.Type {
	case "timeline":
		for _, e := range m.NotificationContainer.TimelineEntry {
			if e.State == 5 && e.Type >= 1 && e.Type <= 4 {
				return true
			}
		}
	case "activity":
		for _, a := range m.NotificationContainer.ActivityNotification {
			if a.Event == "ended" && a.Activity.Type == "library.update.section" {
				return true
			}
		}
	}
	return false
}

// dialWS opens a client WebSocket (RFC 6455) over TLS or plain TCP.
func dialWS(ctx context.Context, u *url.URL) (net.Conn, *bufio.Reader, error) {
	host := u.Host
	if u.Port() == "" {
		host = net.JoinHostPort(u.Hostname(), map[string]string{"https": "443"}[u.Scheme])
	}
	d := &net.Dialer{Timeout: 10 * time.Second}
	var conn net.Conn
	var err error
	if u.Scheme == "https" {
		conn, err = tls.DialWithDialer(d, "tcp", host, &tls.Config{ServerName: u.Hostname()})
	} else {
		conn, err = d.DialContext(ctx, "tcp", host)
	}
	if err != nil {
		return nil, nil, err
	}
	keyb := make([]byte, 16)
	rand.Read(keyb)
	key := base64.StdEncoding.EncodeToString(keyb)
	path := u.RequestURI()
	fmt.Fprintf(conn, "GET %s HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: %s\r\nSec-WebSocket-Version: 13\r\n\r\n", path, u.Host, key)
	r := bufio.NewReader(conn)
	conn.SetReadDeadline(time.Now().Add(15 * time.Second))
	res, err := http.ReadResponse(r, nil)
	if err != nil {
		conn.Close()
		return nil, nil, err
	}
	res.Body.Close()
	if res.StatusCode != http.StatusSwitchingProtocols {
		conn.Close()
		return nil, nil, fmt.Errorf("Plex answered %s", res.Status)
	}
	return conn, r, nil
}

// readFrame reads one (unfragmented or reassembled) frame from the server; server frames aren't masked.
func readFrame(r *bufio.Reader, conn net.Conn) (byte, []byte, error) {
	var op byte
	var out []byte
	for {
		h := make([]byte, 2)
		if _, err := io.ReadFull(r, h); err != nil {
			return 0, nil, err
		}
		fin, code := h[0]&0x80 != 0, h[0]&0x0f
		n := uint64(h[1] & 0x7f)
		switch n {
		case 126:
			b := make([]byte, 2)
			if _, err := io.ReadFull(r, b); err != nil {
				return 0, nil, err
			}
			n = uint64(binary.BigEndian.Uint16(b))
		case 127:
			b := make([]byte, 8)
			if _, err := io.ReadFull(r, b); err != nil {
				return 0, nil, err
			}
			n = binary.BigEndian.Uint64(b)
		}
		if n > 4<<20 {
			return 0, nil, errors.New("message too big")
		}
		var mask []byte
		if h[1]&0x80 != 0 {
			mask = make([]byte, 4)
			if _, err := io.ReadFull(r, mask); err != nil {
				return 0, nil, err
			}
		}
		p := make([]byte, n)
		if _, err := io.ReadFull(r, p); err != nil {
			return 0, nil, err
		}
		for i := range p {
			if mask != nil {
				p[i] ^= mask[i%4]
			}
		}
		if code >= 0x8 { // control frames come whole, even in the middle of a message
			return code, p, nil
		}
		if code != 0 {
			op = code
		}
		out = append(out, p...)
		if fin {
			return op, out, nil
		}
	}
}

// writeFrame sends one masked frame (clients must mask).
func writeFrame(conn net.Conn, op byte, p []byte) error {
	h := []byte{0x80 | op}
	switch {
	case len(p) < 126:
		h = append(h, 0x80|byte(len(p)))
	case len(p) < 65536:
		h = append(h, 0x80|126, byte(len(p)>>8), byte(len(p)))
	default:
		h = append(h, 0x80|127)
		h = binary.BigEndian.AppendUint64(h, uint64(len(p)))
	}
	mask := make([]byte, 4)
	rand.Read(mask)
	h = append(h, mask...)
	b := make([]byte, len(p))
	for i := range p {
		b[i] = p[i] ^ mask[i%4]
	}
	conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	_, err := conn.Write(append(h, b...))
	return err
}
