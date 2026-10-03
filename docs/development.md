# Development

How DAWGit is built, how to build it, and how it is tested. For using DAWGit, see the [README](../README.md).

## Build

Requirements: Go 1.27+, Node.js, the Wails v3 CLI (`wails3`) and, for the installer, NSIS.

```
go build -o dawgit.exe ./cmd/dawgit                       # command line tool (also the team server)
go test ./...                                             # all Go tests
powershell -ExecutionPolicy Bypass -File scripts\build-windows.ps1   # -> dist\DAWGit-<version>-setup.exe
```

The release number lives in `internal/version/version.go`; the build script reads it for the installer name and the file properties of `DAWGit.exe`.

The desktop app is in `desktop/` (Go + Svelte 5 frontend in `desktop/frontend/`). After changing Go methods the frontend calls, regenerate the TypeScript bindings:

```
cd desktop
wails3 generate bindings -ts
cd frontend && npx svelte-check && npm run build
```

To try the desktop UI in a browser (no native dialogs), build it in server mode:

```
cd desktop
go build -tags server -o bin/DAWGit-server.exe ./cmd/dawgit-desktop
set WAILS_SERVER_PORT=8765
set DAWGIT_CONFIG_DIR=%TEMP%\dawgit-dev      # a fresh settings folder (shows onboarding)
set DAWGIT_DEV_PICK_DIR=C:\path\to\a\folder   # what the folder picker returns
bin\DAWGit-server.exe                         # then open http://localhost:8765/
```

## Layout

- `cmd/dawgit` — command line tool (package `cli`; `--json` results and error codes in `cli/output.go`); `dawgit serve` is the team server
- `docs/` — also a Go package: embeds `agents.md` for `dawgit help agents`
- `desktop/` — Windows desktop app (Wails v3, Svelte 5); installer in `desktop/build/windows/installer.nsi`
- `internal/xmltree` — ordered XML tree with byte-exact round-trip of Live's output
- `internal/als` — Live Set model, content fingerprints (noise-aware), structural validator
- `internal/diff` — track-level semantic diff
- `internal/merge` — track-level 3-way merge (tracks, placement, order, sends, globals) with id repair
- `internal/store` — content-addressed blob store (SHA-256)
- `internal/project` — versions, status, history, restore with sample relinking; `sync.go` does save/update/clone and merges
- `internal/teamwatch` — background watcher: backs up unsaved work, teammates' edits (soft locks), new versions
- `internal/livecheck` — detects a running Live before rewriting sets
- `internal/manifest` — version manifest shared by client and server
- `internal/server` — self-hosted team server (files on disk, token auth, branch compare-and-swap)
- `internal/remote` — the `Backend` interface with two implementations: the team server's HTTP API and S3-compatible object storage
- `internal/teams` — per-user team store (`%APPDATA%\DAWGit\teams.json`): team addresses, credentials, project locations
- `internal/version` — release number
- `python/` — the original prototype, kept as the reference implementation
- `SampleProjects/` — Ableton projects used as test fixtures (audio via Git LFS)

## Tests

- `go test ./...` runs everything that needs no network.
- `internal/remote/backendtest` is a contract test every storage backend must pass. It runs against the team server and an in-memory fake S3; to run it against a real bucket:

  ```
  set DAWGIT_TEST_STORAGE=<connection code>
  go test ./internal/remote -run Live -v
  ```

  It has been run against Versity S3 Gateway and Cloudflare R2.
- Merge output is checked in Ableton Live by hand: `python -m tests.make_live_samples` (in `python/`) writes `MergeTest-*.als` files to open in Live.

### Python reference implementation

`python/` holds the original prototype. The Go port must stay output-identical to it:

```
cd python
python -m unittest discover -s tests -t .        # reference tests
python -m tests.make_golden                      # writes ../testdata/golden for Go differential tests
```

`go test ./internal/merge` compares Go merge/diff/validate output byte-for-byte against the golden data (skipped when absent).

## Design notes

- [docs/als-format-notes.md](als-format-notes.md) — findings about the `.als` format
- [docs/design/storage-backends.md](design/storage-backends.md) — pluggable storage backends
- [docs/cli.md](cli.md) — the command line tool
