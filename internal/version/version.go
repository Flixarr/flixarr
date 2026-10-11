// Package version holds build information injected at build time with -ldflags.
//
// Flixarr versions are MAJOR.MINOR.PATCH.BUILD: BUILD counts published releases (it goes up by one per publish and
// never resets). Test deploys between releases carry the last published BUILD and are marked "-dev".
package version

var (
	// Version is MAJOR.MINOR.PATCH.
	Version = "0.1.0"
	// Build is the release counter.
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
