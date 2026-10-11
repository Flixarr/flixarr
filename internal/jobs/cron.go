// Package jobs runs Flixarr's scheduled tasks on editable cron schedules (Settings → Jobs).
package jobs

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Schedule is a parsed 5-field cron expression: minute hour day-of-month month day-of-week.
type Schedule struct {
	min, hour, dom, month, dow [64]bool
	domAny, dowAny             bool
}

var ranges = [5][2]int{{0, 59}, {0, 23}, {1, 31}, {1, 12}, {0, 7}}

func parseField(src string, lo, hi int, set *[64]bool) error {
	for _, part := range strings.Split(src, ",") {
		rng, stepStr, hasStep := strings.Cut(part, "/")
		step := 1
		if hasStep {
			n, err := strconv.Atoi(stepStr)
			if err != nil || n < 1 {
				return fmt.Errorf("%q isn't valid", part)
			}
			step = n
		}
		a, b := lo, hi
		if rng != "*" {
			as, bs, isRange := strings.Cut(rng, "-")
			var err error
			if a, err = strconv.Atoi(as); err != nil {
				return fmt.Errorf("%q isn't valid", part)
			}
			b = a
			if isRange {
				if b, err = strconv.Atoi(bs); err != nil {
					return fmt.Errorf("%q isn't valid", part)
				}
			} else if hasStep {
				b = hi
			}
		}
		if a < lo || b > hi || a > b {
			return fmt.Errorf("%q is out of range (%d–%d)", part, lo, hi)
		}
		for v := a; v <= b; v += step {
			set[v] = true
		}
	}
	return nil
}

// Parse reads a cron expression like "0 3 * * *".
func Parse(expr string) (*Schedule, error) {
	f := strings.Fields(expr)
	if len(f) != 5 {
		return nil, fmt.Errorf("a schedule needs 5 parts: minute hour day month weekday")
	}
	s := &Schedule{domAny: f[2] == "*", dowAny: f[4] == "*"}
	sets := []*[64]bool{&s.min, &s.hour, &s.dom, &s.month, &s.dow}
	for i, src := range f {
		if err := parseField(src, ranges[i][0], ranges[i][1], sets[i]); err != nil {
			return nil, err
		}
	}
	if s.dow[7] { // 7 is also Sunday
		s.dow[0] = true
	}
	return s, nil
}

// Matches reports whether the schedule fires in t's minute.
func (s *Schedule) Matches(t time.Time) bool {
	return s.min[t.Minute()] && s.hour[t.Hour()] && s.month[int(t.Month())] && s.dayOK(t)
}

// Next returns the first time after t the schedule fires (zero if none within ~a year).
func (s *Schedule) Next(t time.Time) time.Time {
	t = t.Truncate(time.Minute).Add(time.Minute)
	for end := t.AddDate(1, 1, 0); t.Before(end); {
		if !s.month[int(t.Month())] {
			t = time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, t.Location())
			continue
		}
		if !s.dayOK(t) {
			t = time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, t.Location())
			continue
		}
		if !s.hour[t.Hour()] {
			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour()+1, 0, 0, 0, t.Location())
			continue
		}
		if s.Matches(t) {
			return t
		}
		t = t.Add(time.Minute)
	}
	return time.Time{}
}

// dayOK applies standard cron day rules: when both day fields are set, either may match.
func (s *Schedule) dayOK(t time.Time) bool {
	domOK, dowOK := s.dom[t.Day()], s.dow[int(t.Weekday())]
	switch {
	case s.domAny && s.dowAny:
		return true
	case s.domAny:
		return dowOK
	case s.dowAny:
		return domOK
	}
	return domOK || dowOK
}
