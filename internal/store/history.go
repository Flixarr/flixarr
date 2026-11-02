package store

import (
	"encoding/json"
	"sort"
	"strconv"
	"time"
)

// The notes a Seerr import gives the requests it brings in as already ended. Those steps are stored as
// "expired" but nothing left Plex; they read as "imported" (#186).
const (
	SeerrNotFound   = "Not in Plex, Radarr or Sonarr when it was brought over from Seerr."
	SeerrAskedAgain = "Requested again later."
	SeerrDuplicate  = "Already requested in Flixarr."
)

// EndedOnImport: a request the Seerr import brought in as already ended (its note says why).
func EndedOnImport(status, note string) bool {
	return status == "expired" && (note == SeerrNotFound || note == SeerrAskedAgain || note == SeerrDuplicate)
}

// eventStatus is how a stored step reads: an import's "expired" is "imported", not a removal.
func eventStatus(status, note string) string {
	if EndedOnImport(status, note) {
		return "imported"
	}
	return status
}

// HistoryItem is one step in a title's history: a request's step, an issue reported or resolved, or a
// title event (added to Plex, Redownload, deleted, data cleared).
type HistoryItem struct {
	At     string `json:"at"`
	Kind   string `json:"kind"`   // request:<status>, issue, resolved, plex, redownload, queuefix, fix, deleted, cleared
	Note   string `json:"note"`   // the reason, the issue's kind, what was deleted
	Who    string `json:"who"`    // who did it, when known
	Detail string `json:"detail"` // series: what the request asked for
	seq    int    // the order it was read in, for steps saved in the same second
}

// AddTitleEvent records something done to a title (userID 0 = nobody in particular).
func (s *Store) AddTitleEvent(typ string, tmdbID int, kind, note string, userID int64, at string) {
	if at == "" {
		at = now()
	}
	var who any
	if userID > 0 {
		who = userID
	}
	s.db.Exec(`INSERT INTO title_events (type, tmdb_id, kind, note, user_id, at) VALUES (?, ?, ?, ?, ?, ?)`, typ, tmdbID, kind, note, who, at)
}

// TitleHistory is everything that happened to a title, newest first.
func (s *Store) TitleHistory(typ string, tmdbID int) ([]HistoryItem, error) {
	out := []HistoryItem{}
	add := func(q string, row func(scan func(...any) error) (HistoryItem, error)) error {
		rows, err := s.db.Query(q, typ, tmdbID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			h, err := row(rows.Scan)
			if err != nil {
				return err
			}
			out = append(out, h)
		}
		return rows.Err()
	}
	// Request steps (Fix steps come from the title events instead); "requested" names the person who asked, approved/declined name who decided.
	err := add(`SELECT e.status, e.note, e.at, COALESCE(u.username, ''), r.mode, r.seasons FROM request_events e
		JOIN requests r ON r.id = e.request_id LEFT JOIN users u ON u.id = r.user_id WHERE r.type = ? AND r.tmdb_id = ? AND e.status NOT IN ('redownload', 'fix') ORDER BY e.id`,
		func(scan func(...any) error) (HistoryItem, error) {
			var h HistoryItem
			var asker, mode, seasons string
			err := scan(&h.Kind, &h.Note, &h.At, &asker, &mode, &seasons)
			h.Kind = eventStatus(h.Kind, h.Note)
			if h.Kind == "requested" {
				h.Who = asker
			}
			if h.Kind == "approved" || h.Kind == "declined" {
				h.Who, h.Note = h.Note, ""
			}
			if typ == "series" && h.Kind == "requested" {
				h.Detail = askedFor(mode, seasons)
			}
			h.Kind = "request:" + h.Kind
			return h, err
		})
	if err == nil {
		err = add(`SELECT i.kind, i.sub, i.status, i.created_at, i.updated_at, COALESCE(u.username, ''), COALESCE(v.username, '') FROM issues i
			LEFT JOIN users u ON u.id = i.user_id LEFT JOIN users v ON v.id = i.resolved_by WHERE i.type = ? AND i.tmdb_id = ?`,
			func(scan func(...any) error) (HistoryItem, error) {
				var kind, sub, status, created, updated, by, resolver string
				err := scan(&kind, &sub, &status, &created, &updated, &by, &resolver)
				note := kind
				if sub != "" {
					note += ": " + sub
				}
				if status == "resolved" {
					out = append(out, HistoryItem{At: updated, Kind: "resolved", Note: note, Who: resolver})
				}
				return HistoryItem{At: created, Kind: "issue", Note: note, Who: by}, err
			})
	}
	if err == nil {
		err = add(`SELECT e.kind, e.note, e.at, COALESCE(u.username, '') FROM title_events e LEFT JOIN users u ON u.id = e.user_id WHERE e.type = ? AND e.tmdb_id = ? ORDER BY e.id`,
			func(scan func(...any) error) (HistoryItem, error) {
				var h HistoryItem
				return h, scan(&h.Kind, &h.Note, &h.At, &h.Who)
			})
	}
	// Newest first. Steps saved in the same second (a Seerr import writes Requested and Approved together)
	// keep the order they happened in: the one read later is the newer.
	for i := range out {
		out[i].seq = i
	}
	sort.SliceStable(out, func(a, b int) bool {
		ta, tb := when(out[a].At), when(out[b].At)
		if ta.Equal(tb) {
			return out[a].seq > out[b].seq
		}
		return ta.After(tb)
	})
	return out, err
}

// when reads a stored time (RFC 3339, with or without fractions).
func when(s string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, s)
	return t
}

// askedFor says what a series request asked for: "Whole show", "Season 2", "Seasons 1, 3".
func askedFor(mode, seasons string) string {
	if mode == "all" {
		return "Whole show"
	}
	if mode == "episodes" {
		return "Some episodes"
	}
	var list []int
	json.Unmarshal([]byte(seasons), &list)
	return SeasonsText(list)
}

// SeasonsText is "Season 2" or "Seasons 1, 3" ("" for none).
func SeasonsText(list []int) string {
	switch len(list) {
	case 0:
		return ""
	case 1:
		return "Season " + strconv.Itoa(list[0])
	}
	s := "Seasons "
	for i, n := range list {
		if i > 0 {
			s += ", "
		}
		s += strconv.Itoa(n)
	}
	return s
}

// TitleEventSince says whether a title had an event of this kind after a time.
func (s *Store) TitleEventSince(typ string, tmdbID int, kind, after string) bool {
	var n int
	s.db.QueryRow(`SELECT COUNT(*) FROM title_events WHERE type = ? AND tmdb_id = ? AND kind = ? AND at > ?`, typ, tmdbID, kind, after).Scan(&n)
	return n > 0
}

// AddFixSteps puts a Fix action ("Searched again", "Got a different release") on the timeline of each of a
// title's requests still in play, or only on request reqID when it's set, with who did it. It returns the
// steps, so they can be taken back with DropFixSteps if the action fails.
func (s *Store) AddFixSteps(typ string, tmdbID int, reqID int64, text string, userID int64) []int64 {
	var who string
	s.db.QueryRow(`SELECT username FROM users WHERE id = ?`, userID).Scan(&who)
	if who != "" {
		text += " by " + who
	}
	q, args := `SELECT id FROM requests WHERE type = ? AND tmdb_id = ? AND status IN (`+inList(append([]string{"failed"}, OpenStatuses...))+`)`, []any{typ, tmdbID}
	if reqID > 0 {
		q, args = `SELECT id FROM requests WHERE id = ?`, []any{reqID}
	}
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil
	}
	var reqs []int64
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			reqs = append(reqs, id)
		}
	}
	rows.Close()
	t := now()
	var out []int64
	for _, id := range reqs {
		if res, err := s.db.Exec(`INSERT INTO request_events (request_id, status, note, at) VALUES (?, 'fix', ?, ?)`, id, text, t); err == nil {
			n, _ := res.LastInsertId()
			out = append(out, n)
		}
	}
	return out
}

// DropFixSteps takes back steps from AddFixSteps (the action they describe didn't work).
func (s *Store) DropFixSteps(ids []int64) {
	for _, id := range ids {
		s.db.Exec(`DELETE FROM request_events WHERE id = ? AND status = 'fix'`, id)
	}
}
