// Package version holds build information injected at build time with -ldflags.
//
// Flixarr versions are MAJOR.MINOR.PATCH.BUILD: BUILD is the commit count and never resets.
package version

var (
	// Version is MAJOR.MINOR.PATCH.
	Version = "0.1.0"
	// Build is the build counter (git commit count).
	Build = "0"
	// Commit is the short git commit the binary was built from.
	Commit = "dev"
	// Channel is the update channel: "nightly" or "stable".
	Channel = "nightly"
)

// Full returns the four-part version, e.g. 0.1.0.42.
func Full() string { return Version + "." + Build }
