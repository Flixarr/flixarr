package requests

import (
	"errors"
	"strings"
)

// Problem is an error with a plain version for people who don't manage requests: no Radarr, Sonarr or
// Plex names and no codes. Error() is the full text for people who do.
type Problem struct {
	Plain string
	Err   error
}

func (p *Problem) Error() string { return p.Err.Error() }
func (p *Problem) Unwrap() error { return p.Err }

// plain is an error whose text is fine for everyone.
func plain(msg string) error {
	return &Problem{Plain: msg, Err: errors.New(msg)}
}

// Plain is err in words for people who don't manage requests: its Problem text, or fallback.
func Plain(err error, fallback string) string {
	var p *Problem
	if errors.As(err, &p) && p.Plain != "" {
		return p.Plain
	}
	return fallback
}

// sentences joins notes: one note stands alone without a period, several read as sentences.
func sentences(notes []string) string {
	if len(notes) == 1 {
		return strings.TrimSuffix(notes[0], ".")
	}
	out := make([]string, len(notes))
	for i, n := range notes {
		if n != "" && !strings.HasSuffix(n, ".") {
			n += "."
		}
		out[i] = n
	}
	return strings.Join(out, " ")
}
