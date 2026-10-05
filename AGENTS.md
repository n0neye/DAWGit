# AGENTS.md

For AI agents working on DAWGit's code. To **use** DAWGit in a project
(save, update, merge), read [docs/agents.md](docs/agents.md) instead, or run
`dawgit help agents`.

## The project

DAWGit is version control for creative projects (Ableton Live sets, Unity
and Unreal projects): a Go command line tool (`cmd/dawgit`, package `cli`)
and a Windows desktop app (`desktop/`, Wails v3 + Svelte 5). A private build
adds extensions through `ext/`; keep `cli` and `desktop` usable as libraries
(no `os.Exit` outside `cli.Main`).

Two release lines come from `main`: Stable, and Nightly (`-tags nightly`),
which adds the project kinds still in testing from `presets/`. Anything
that changes what a team stores must be a team feature (see
[docs/design/channels.md](docs/design/channels.md)).

[docs/development.md](docs/development.md) has the layout and how to build;
`docs/design/` has the design notes (storage backends, tree manifests,
chunked big files).

## Build and test

```sh
go build ./...
go vet ./...
go test ./...                          # no network needed
go test -tags nightly ./...            # the Nightly build too
cd desktop/frontend && npx svelte-check   # after any UI change
```

- After changing Go methods the frontend calls: `wails3 generate bindings
  -ts` in `desktop/`, then check `git diff -w` on the bindings before
  committing.
- `testdata/golden` pins the output of diff and merge byte for byte
  (`go test ./internal/merge`); change it only on purpose.
- The self-hosted team server (`dawgit serve`, `internal/server`) is frozen:
  it keeps working, but new features go to storage teams (and the coming
  hosted service), not to it.

## Conventions

- Code, comments, identifiers, commit messages: English. Plain words in
  messages users see; the app translates its UI (`desktop/frontend/src/lib/
  locales`, English text is the key: `node scripts/i18n-check.mjs` in
  `desktop/frontend` lists what is missing).
- Match the surrounding code: short doc comments that say why, no
  boilerplate.
- `go vet` and `gofmt` clean.
- The CLI's `--json` output and error codes are an interface (see
  `cli/output.go` and docs/agents.md): add fields freely, don't rename or
  remove them, and keep the codes stable.

## Things that must not break

- **Stored data is forever.** Content hashes, the blob header
  (`internal/blob`), chunk boundaries (`internal/chunk`, golden test) and
  version formats are read by every DAWGit after; a change that writes
  something older versions can't read needs a release that forces updating
  (`-MinVersion`).
- **Never lose a user's file.** Anything that rewrites project files checks
  first (Live running, unsaved changes) and fails rather than guesses.
- **Secrets.** Connection codes and storage keys never go into logs, test
  output, commits or docs. The release signing key is never in the repo.
- Storage cleanup (`internal/remote/gc.go`) must never delete something a
  version, a share in progress or a chunk list relies on.
