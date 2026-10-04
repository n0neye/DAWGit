//go:build !nightly

package version

// Channel is the release line this build belongs to: "stable", or "nightly"
// for builds with the nightly tag (main as it is, with the features still
// in testing: Unity, Unreal…).
const Channel = "stable"
