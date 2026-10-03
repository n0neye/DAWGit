// Package docs holds the guides the CLI prints (dawgit help agents).
package docs

import _ "embed"

// Agents is agents.md: how AI agents use DAWGit.
//
//go:embed agents.md
var Agents string

// AgentsSnippet is agents-snippet.md: a few lines for a project's AGENTS.md.
//
//go:embed agents-snippet.md
var AgentsSnippet string
