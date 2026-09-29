# DAWGit

Version control and collaboration tool for music production, targeting Ableton Live users first.

## Scope

- Small teams working on the same songs, with a self-hosted server
- Project sync & remote multi-user editing
- Sample management via content-addressed storage (dedupe, lazy fetch, path relinking)
- Track-level semantic 3-way merge of `.als` Live Sets (L2)

## Layout

- `SampleProjects/` — real Ableton projects used as test fixtures (audio stored via Git LFS)

## Install (Windows)

See [docs/team-setup.md](docs/team-setup.md). Build the installer with:

```
powershell -ExecutionPolicy Bypass -File scripts\build-windows.ps1   # -> dist\DAWGit-<version>-setup.exe
```

Needs Go, Node.js, Wails v3 CLI (`wails3`) and NSIS. The version number lives in `internal/version/version.go`.

## Build & test (Go)

Requires Go 1.27+.

```
go build -o dawgit.exe ./cmd/dawgit
dawgit info <set.als>                            # tracks, devices, clips, automation, plugins, samples
dawgit diff <a.als> <b.als>                      # semantic diff
dawgit merge <base> <ours> <theirs> -o out.als [--strategy fail|ours|theirs|both]
go test ./...
```

Team workflow:

```
dawgit serve --data D:\dawgit-data            # on any team machine or NAS; prints the token
dawgit init && dawgit remote http://host:7331 --token T && dawgit save -m "first version"
dawgit clone http://host:7331 "Song" --token T  # other members
dawgit save -m "added drums"                   # save a version and share it (merges others' versions first)
dawgit update                                  # get the team's latest versions
```

Keep `dawgit agent` running while you work: it backs up your unsaved sets to the server, shows which tracks teammates are editing (track-level soft locks, with a warning when you edit the same track), and tells you when someone saves a new version. It never changes your files; `dawgit status` shows the same information.

Branches (advanced teams):

```
dawgit branch                    # list branches and their latest versions
dawgit branch new yi-ideas       # start a branch from your current version
dawgit switch main
dawgit merge yi-ideas --preview  # what would come in (semantic diff) and what conflicts
dawgit merge yi-ideas            # merge into your branch and share
dawgit update --preview          # what the team changed, without applying it
```

`save`/`update` merge Live Sets track by track; when you and others changed the same track (or the same sample file) they stop and ask for `--strategy ours|theirs|both`. They refuse to rewrite sets while Ableton Live is running if others' changes must be merged in.

Local project workflow (inside an Ableton project folder):

```
dawgit init [--author NAME]        # creates .dawgit/
dawgit snapshot -m "message"       # record .als files, Samples/ and external samples
dawgit status                      # changed files; semantic diff for modified sets
dawgit log
dawgit checkout <id|HEAD~N> [--force]   # restore and relink samples for this machine
```

Snapshots store file contents by SHA-256 in `.dawgit/objects` (deduplicated). Samples referenced from outside the project are stored too and, on a machine that lacks them, materialized under `.dawgit/external/` with the set's sample paths rewritten. Samples from Live packs are only recorded by pack name. `Backup/` and `*.asd` are ignored.

- `internal/xmltree` — ordered XML tree with byte-exact round-trip of Live's output
- `internal/als` — Live Set model, content fingerprints (noise-aware), structural validator
- `internal/diff` — track-level semantic diff
- `internal/merge` — track-level 3-way merge (tracks, placement, order, sends, globals) with id repair
- `internal/store` — content-addressed blob store
- `internal/project` — snapshots, status, log, checkout with sample relinking
- `internal/livecheck` — detects a running Live before rewriting sets
- `internal/manifest` — snapshot manifest shared by client and server
- `internal/server` — self-hosted server (files on disk, token auth, branch compare-and-swap)
- `internal/remote` — HTTP client; `internal/project/sync.go` does save/update/clone and snapshot merges

## Python reference implementation

`python/` holds the original prototype. The Go port must stay output-identical to it:

```
cd python
python -m unittest discover -s tests -t .        # reference tests
python -m tests.make_golden                      # writes ../testdata/golden for Go differential tests
python -m tests.make_live_samples                # MergeTest-*.als to open in Live
```

`go test ./internal/merge` compares Go merge/diff/validate output byte-for-byte against the golden data (skipped when absent).

See [docs/als-format-notes.md](docs/als-format-notes.md) for findings about the .als format, and [docs/design/](docs/design/) for design proposals (e.g. [pluggable storage backends](docs/design/storage-backends.md)).
