// Package version holds build information injected at build time with -ldflags.
//
// Flixarr versions are MAJOR.MINOR.PATCH.BUILD: BUILD is the git commit count, so it never resets and maps to
// exactly one commit. Unpublished test deploys are marked "-dev".
package version

var (
	// Version is MAJOR.MINOR.PATCH.
	Version = "0.1.0"
	// Build is the git commit count.
	Build = "0"
	// Commit is the short git commit the binary was built from.
	Commit = "dev"
	// Channel is the update channel: "nightly" or "stable".
	Channel = "nightly"
	// Dev is set for test deploys that haven't been published.
	Dev = ""
)

// Full returns the four-part version, e.g. 0.1.0.42, or 0.1.0.42-dev for an unpublished test deploy.
func Full() string {
	if Dev != "" {
		return Version + "." + Build + "-dev"
	}
	return Version + "." + Build
}
