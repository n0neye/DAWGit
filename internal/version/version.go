// Package version holds the DAWGit release number. The build script reads it
// from here too (installer, Windows file properties).
package version

// Version is the release this build is (Stable) or leads up to (Nightly).
const Version = "0.13.4"

// Build marks a Nightly build: "nightly.<UTC time>" (the build script sets
// it with -ldflags -X), "" for a Stable one.
var Build string

// Full is the version with its build: "0.13.4" or "0.13.0-nightly.202610041530".
func Full() string {
	if Build == "" {
		return Version
	}
	return Version + "-" + Build
}

// Edition names a build with extensions (e.g. "Pro"; set through
// ext.SetEdition); "" for the public app. It is shown next to the version
// and the name, and such builds don't offer the public app's updates.
var Edition string

// Name is the app's name, with the edition ("DAWGit Pro").
func Name() string {
	if Edition == "" {
		return "DAWGit"
	}
	return "DAWGit " + Edition
}

// Display is the version with the edition ("0.6.1 Pro").
func Display() string {
	if Edition == "" {
		return Full()
	}
	return Full() + " " + Edition
}
