// Package testinstall marks a test install: a second Flixarr (the one being developed on) that shares Plex,
// Radarr, Sonarr and Maintainerr with the install people really use. Turned on with FLIXARR_TEST_INSTALL=true
// on the container, never in the database, so a copied database never carries it to or from the real install.
//
// On a test install Flixarr doesn't send automatic notifications (Send test still works), write to Maintainerr,
// request from Plex Watchlists, push quality profiles or webhooks into Radarr and Sonarr, or check for
// updates; it shows a TEST badge, and admins can view the app as another role.
package testinstall

import (
	"errors"
	"os"
)

// On reports whether this is a test install (read once at start; tests may set it).
var On = os.Getenv("FLIXARR_TEST_INSTALL") == "true"

// ErrBlocked is returned by writes a test install doesn't make.
var ErrBlocked = errors.New("not done from a test install (it shares its servers with the real one)")
