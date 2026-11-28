package requests

import (
	"fmt"
	"strings"
	"time"

	"github.com/Flixarr/flixarr/internal/store"
)

// How a request shows (#301): the server decides its state and the one line under it, and every page
// (the Requests list, the request window, title pages) only draws them. Managers get the real reason;
// everyone else gets it in plain words. Lines say what happened and stop (Rule 40).

// State is a request's status as shown: the stored status, except Requested splits into Not out yet (#300).
func State(r store.Request) string {
	switch r.Status {
	case "approved", "sent":
		if r.Unreleased {
			return "notout"
		}
		return "requested"
	}
	return r.Status
}

// waiting: states still on their way, where a server not responding changes the line.
func waiting(state string) bool {
	switch state {
	case "notout", "requested", "grabbed", "downloading", "importing", "stuck", "partial":
		return true
	}
	return false
}

// View fills in State, Hint, HintTone and BrokeAt. down names the app that isn't responding ("Sonarr"),
// "" when it's fine: the status stays what it was and only the line says so.
func (s *Service) View(r store.Request, admin bool, down string) store.Request {
	r.State = State(r)
	r.Hint, r.HintTone, r.BrokeAt = "", "", ""
	pick := func(manager, everyone string) string {
		if admin {
			return manager
		}
		return everyone
	}
	when := Day(r.UpdatedAt)
	switch r.State {
	case "notout":
		switch {
		case r.OutDate == "":
			r.Hint = "No release date yet"
		case r.Type == "series":
			r.Hint = "Airs " + Day(r.OutDate)
		default:
			r.Hint = "Out to download " + Day(r.OutDate)
		}
	case "requested":
		r.Hint = "Looking for a download"
	case "grabbed":
		r.Hint = "Download found, starting soon"
	case "importing":
		r.Hint = pick("Downloaded, waiting for Plex", "Downloaded, almost ready to watch")
	case "stuck":
		plain := "The download hit a problem"
		if r.SomeReady {
			plain = "Some episodes are ready. A download hit a problem."
		}
		r.Hint, r.HintTone = pick(r.Error, plain), "red"
		r.BrokeAt = "download"
		if r.Progress >= 100 {
			r.BrokeAt = "import" // downloaded; going into Plex is what's stuck
		}
	case "partial":
		if r.Progress > 0 {
			r.Hint = "The rest is downloading"
		}
	case "available":
		r.Hint = "Ready to watch on Plex"
	case "failed":
		r.Hint, r.HintTone, r.BrokeAt = pick(r.Error, "Couldn't be requested"), "red", "send"
	case "declined":
		if r.Error != "" {
			r.Hint = "Declined: " + r.Error
		}
	case "expired":
		if store.EndedOnImport(r.Status, r.Error) {
			r.Hint = pick(r.Error, "")
			break
		}
		r.Hint = pick("Removed from Plex on "+when+" by Maintainerr", "Removed from Plex on "+when)
	case "removed":
		r.Hint = s.removedLine(r, when, admin)
	}
	if down != "" && waiting(r.State) {
		r.Hint, r.HintTone = pick("Can't get status ("+down+" isn't responding)", "Problem getting the latest status"), ""
	}
	return r
}

// removedLine says when (and, for managers, how) a request's title was removed.
func (s *Service) removedLine(r store.Request, when string, admin bool) string {
	kind := ArrName(r.Type)
	everyone := "Removed on " + when
	if s.Store.WasAvailable(r.ID) {
		everyone = "Removed from Plex on " + when
	}
	if !admin {
		return everyone
	}
	switch {
	case strings.HasSuffix(r.Error, "outside Flixarr"):
		return "Removed from " + kind + " on " + when + ", outside Flixarr"
	case r.Error == "Removed from "+kind+".": // Delete in Flixarr
		if who := s.Store.TitleEventBy(r.Type, r.TmdbID, "deleted"); who != "" {
			return "Deleted on " + when + " by " + who
		}
		return "Deleted on " + when
	case r.Error == "Removed from Plex and "+kind+".":
		return "Removed from Plex and " + kind + " on " + when
	case r.Error != "":
		return strings.TrimSuffix(r.Error, ".") + " (" + when + ")"
	}
	return everyone
}

// Day is a date as the app writes it in lines: "Nov 2nd", with the year when it isn't this year
// ("Nov 2nd, 2025"). It takes "2026-11-02" or a full timestamp.
func Day(v string) string {
	var t time.Time
	var err error
	if len(v) == 10 {
		t, err = time.ParseInLocation("2006-01-02", v, time.Local)
	} else {
		t, err = time.Parse(time.RFC3339, v)
		t = t.Local()
	}
	if err != nil {
		return ""
	}
	out := t.Format("Jan ") + fmt.Sprint(t.Day()) + suffix(t.Day())
	if t.Year() != time.Now().Year() {
		out += fmt.Sprintf(", %d", t.Year())
	}
	return out
}

func suffix(d int) string {
	if d >= 11 && d <= 13 {
		return "th"
	}
	switch d % 10 {
	case 1:
		return "st"
	case 2:
		return "nd"
	case 3:
		return "rd"
	}
	return "th"
}

// TimeLeft is a download's time left as Radarr/Sonarr give it ("01:12:05", "1.02:00:00") in words.
func TimeLeft(v string) string {
	days, rest := 0, v
	if i := strings.Index(v, "."); i > 0 && i < strings.Index(v, ":") {
		fmt.Sscan(v[:i], &days)
		rest = v[i+1:]
	}
	var h, m, sec int
	if n, _ := fmt.Sscanf(rest, "%d:%d:%d", &h, &m, &sec); n < 2 {
		return ""
	}
	h += days * 24
	switch {
	case h >= 2:
		return fmt.Sprintf("About %d hours left", h)
	case h == 1:
		return "About an hour left"
	case m >= 2:
		return fmt.Sprintf("About %d minutes left", m)
	}
	return "Almost done"
}
