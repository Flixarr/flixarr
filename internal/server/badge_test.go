package server

import (
	"testing"

	"github.com/Flixarr/flixarr/internal/testinstall"
	"github.com/Flixarr/flixarr/internal/version"
)

func TestBuildBadge(t *testing.T) {
	on, pre := testinstall.On, version.Pre
	t.Cleanup(func() { testinstall.On, version.Pre = on, pre })
	for _, c := range []struct {
		test bool
		pre  string
		want string
	}{
		{true, "dev", "DEV"},
		{true, "nightly", "DEV"},
		{false, "nightly", "BETA"},
		{false, "", ""},
		{false, "dev", ""},
	} {
		testinstall.On, version.Pre = c.test, c.pre
		if got := buildBadge(); got != c.want {
			t.Errorf("test install %v, pre %q: got %q, want %q", c.test, c.pre, got, c.want)
		}
	}
}
