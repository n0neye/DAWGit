// Package presets registers the project kinds that are still in testing
// (Unity, Unreal, code, design files): the Nightly channel's builds import
// it (see cmd/dawgit/nightly.go); Stable builds leave them out.
package presets

import (
	_ "dawgit/presets/code"
	_ "dawgit/presets/design"
	_ "dawgit/presets/unity"
	_ "dawgit/presets/unreal"
)
