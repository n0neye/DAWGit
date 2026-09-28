# DAWGit

Version control and collaboration tool for music production, targeting Ableton Live users first.

## Scope (initial)

- Target: small teams (2-3 people), self-hosted first, SaaS later
- Project sync & remote multi-user editing
- Sample management via content-addressed storage (dedupe, lazy fetch, path relinking)
- Track-level semantic 3-way merge of `.als` Live Sets (L2)

## Layout

- `SampleProjects/` — real Ableton projects used as test fixtures (audio stored via Git LFS)

## Build & test (Go)

Requires Go 1.27+.

```
go build -o dawgit.exe ./cmd/dawgit
dawgit info <set.als>                            # tracks, devices, clips, automation, plugins, samples
dawgit diff <a.als> <b.als>                      # semantic diff
dawgit merge <base> <ours> <theirs> -o out.als [--strategy fail|ours|theirs|both]
go test ./...
```

Project workflow (inside an Ableton project folder):

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

## Python reference implementation

`python/` holds the original prototype. The Go port must stay output-identical to it:

```
cd python
python -m unittest discover -s tests -t .        # reference tests
python -m tests.make_golden                      # writes ../testdata/golden for Go differential tests
python -m tests.make_live_samples                # MergeTest-*.als to open in Live
```

`go test ./internal/merge` compares Go merge/diff/validate output byte-for-byte against the golden data (skipped when absent).

See [docs/als-format-notes.md](docs/als-format-notes.md) for findings about the .als format.
