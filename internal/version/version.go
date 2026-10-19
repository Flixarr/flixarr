// Package version holds build information injected at build time with -ldflags.
//
// Versions are MAJOR.MINOR.PATCH. A stable release is just that (0.2.0). Builds on the way to a release add
// what kind of build they are and the git commit count: 0.2.0-dev.245 (a test deploy), 0.2.0-nightly.245.
// The release scripts work the number out from the last release and the issues closed since; it's never
// typed by hand.
package version

var (
	// Version is MAJOR.MINOR.PATCH: the release, or the release a build is on its way to.
	Version = "0.0.0"
	// Pre is "" for a stable release, or the kind of build: "dev" (test deploy) or "nightly".
	Pre = "dev"
	// Build is the git commit count.
	Build = "0"
	// Commit is the short git commit the binary was built from.
	Commit = "dev"
	// Channel is the update channel: "stable" or "nightly" (test builds follow nightly).
	Channel = "nightly"
)

// Full returns the version as people see it: 0.2.0 for a release, 0.2.0-dev.245 for a build on its way.
func Full() string {
	if Pre == "" {
		return Version
	}
	return Version + "-" + Pre + "." + Build
}
