# DAWGit

Version control and collaboration tool for music production, targeting Ableton Live users first.

## Scope (initial)

- Target: small teams (2-3 people), self-hosted first, SaaS later
- Project sync & remote multi-user editing
- Sample management via content-addressed storage (dedupe, lazy fetch, path relinking)
- Track-level semantic 3-way merge of `.als` Live Sets (L2)

## Layout

- `SampleProjects/` — real Ableton projects used as test fixtures (audio stored via Git LFS)

## Prototype (Python, stdlib only)

```
python -m dawgit info <set.als>                  # tracks, devices, clips, plugins, samples
python -m dawgit diff <a.als> <b.als>            # semantic diff
python -m dawgit merge <base> <ours> <theirs> -o out.als [--strategy fail|ours|theirs|both]
python -m unittest discover -s tests -t .        # tests
python -m tests.make_live_samples                # write MergeTest-*.als to open in Live
```

- `dawgit/als.py` — byte-exact load/save and model access
- `dawgit/normalize.py` — content fingerprints ignoring save-to-save noise
- `dawgit/diff.py` — track-level semantic diff
- `dawgit/merge.py` — track-level 3-way merge (tracks, per-send, global sections) with id repair
- `dawgit/validate.py` — structural invariants a set must satisfy

See [docs/als-format-notes.md](docs/als-format-notes.md) for findings about the .als format.
