package jobs

import (
	"testing"
	"time"
)

func TestNext(t *testing.T) {
	loc := time.UTC
	from := time.Date(2026, 10, 11, 14, 7, 30, 0, loc) // a Sunday
	cases := map[string]time.Time{
		"0 3 * * *":    time.Date(2026, 10, 12, 3, 0, 0, 0, loc),
		"*/15 * * * *": time.Date(2026, 10, 11, 14, 15, 0, 0, loc),
		"0 5 * * 0":    time.Date(2026, 10, 18, 5, 0, 0, 0, loc),
		"30 9 1 * *":   time.Date(2026, 11, 1, 9, 30, 0, 0, loc),
		"0 0 1 1 *":    time.Date(2027, 1, 1, 0, 0, 0, 0, loc),
		"0 12 * * 1-5": time.Date(2026, 10, 12, 12, 0, 0, 0, loc),
		"8 14 * * 7":   time.Date(2026, 10, 11, 14, 8, 0, 0, loc),
	}
	for expr, want := range cases {
		s, err := Parse(expr)
		if err != nil {
			t.Fatalf("%s: %v", expr, err)
		}
		if got := s.Next(from); !got.Equal(want) {
			t.Errorf("%s: got %v, want %v", expr, got, want)
		}
	}
	for _, bad := range []string{"", "* * * *", "60 * * * *", "* 24 * * *", "*/0 * * * *", "a * * * *", "5-1 * * * *"} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("%q should fail", bad)
		}
	}
}
