# Tree manifests (version format 2)

Status: proposal, not built.

## Why

A version record (manifest) lists every file of the project. That is fine for
a song (a few hundred files) and expensive for a game project:

| 100,000-file project       | today                       |
|----------------------------|-----------------------------|
| one version record         | 16.5 MB                     |
| 31 versions on disk        | 512 MB of records           |
| a commit changing 10 files | writes (and shares) 16.5 MB |
| comparing two versions     | reads both whole records    |

Most of every record repeats the one before it. Stage 1 of the scale work
(0.7.x) made scanning, status and the UI fast without touching the format;
what's left is the size of the records themselves.

## The idea

Like git: each folder is its own small record (a *tree*) listing its files
and its subfolders' trees, named by the hash of its content. A version
points to the tree of the project folder. A commit writes new trees only for
folders that changed, and the folders above them; everything else is shared
with the previous version.

A commit that changes 10 files in a 100,000-file project writes about 30
trees (each folder on the way down), typically tens of KB in all, instead of
16.5 MB. Comparing two versions skips every folder whose tree hash is the
same on both sides.

## Format

### Version record, format 2

```json
{
  "version": 2,
  "parents": ["…"],
  "author": "…", "author_id": "…", "time": "…", "message": "…",
  "tree": "<hash of the project folder's tree>",
  "files": 100031,
  "size": 536870912,
  "external": [ … ], "packs": [ … ], "missing": [ … ]
}
```

- `tree` replaces `files`. `files` and `size` (count and bytes of all
  files) are for display without reading the trees.
- `external`, `packs` and `missing` stay as they are (Live sets' samples
  outside the project: few, and not in any folder of it).
- The id is still the SHA-256 of the record's bytes.

### Tree

Text, one line per entry, sorted by name (byte order), `\n` after every
line:

```
d <tree hash> <name>
f <content hash> <size> <name>
```

- The name is the rest of the line (it may contain spaces). Names with a
  newline can't be stored; Windows doesn't allow them, and a commit with one
  on macOS fails with a clear message.
- An empty folder isn't stored (as today: only files are tracked).
- The tree's hash is the SHA-256 of its bytes, like file contents.

Text rather than JSON: about half the size, and quick to read and write.

## Where trees live

- **Team storage:** with file contents, under `objects/<ab>/<rest>`. They
  are content-addressed like files, so they go up with the same batched
  "which are missing" check and parallel uploads, and are shared between
  versions, branches and projects.
- **This computer:** `.dawgit/trees/<ab>/<rest>`, apart from file contents.
  Trees are never pruned: they are the history. File contents are pruned as
  today. GC (unreferenced cleanup) follows trees.
- **Memory:** parsed trees are cached by hash. Versions share nearly all
  their trees, so reading the 31st version of a project costs only the few
  trees that changed since the 30th.

## Safety: old clients must not misread a format-2 version

Today no client checks a version record's `version` field. A 0.7 client
reading a format-2 record would see no `files`: **an empty project**, and
taking it in would remove every file. So format-2 records must be
unreadable to old clients, not merely different:

- Their key is `projects/<pid>/snapshots/<id>.v2.json` (and
  `.dawgit/snapshots/<id>.v2.json` locally). An old client asking for
  `<id>.json` gets "not found" and stops, with nothing changed.
- From now on, clients check `version` and refuse records newer than they
  understand ("This project needs DAWGit 0.9 or later"), so the next format
  change can be a normal one.
- The project's `project.json` in team storage gets `"format": 2` once its
  first format-2 version is shared. Clients that know the field explain the
  upgrade instead of failing on a missing record.

## Rollout

1. **0.8 reads format 2, still writes format 1.** It also checks `version`
   and the project's `format`. Teammates update at their own pace; nothing
   changes for anyone yet.
2. **0.9 writes format 2** for:
   - projects created with 0.9 or later, and
   - existing projects when someone chooses **Upgrade project format** in the
     project's menu. The app first lists team members whose app is older
     (from their workspace records) and warns that they'll need to update.

   Songs and other small projects can stay on format 1 for good. Both
   formats are read forever, and a format-2 version may have a format-1
   parent.

Old versions are never rewritten: their ids, and every branch pointing at
them, stay valid.

## What changes in the code

- `manifest`: `Tree`, `FileCount`, `TotalSize`; tree encode/parse; a
  `Files(load)` accessor that flattens trees (through the cache) so most
  callers keep working on a flat list during the move.
- `project`:
  - Snapshot builds trees bottom-up from the scanned files, reusing the
    previous version's tree for every unchanged folder.
  - Status, merge and checkout compare trees first and descend only into
    folders whose hashes differ.
  - History and ancestry already use headers only (stage 1).
- `sync`: upload and download the trees a version needs that the other
  side doesn't have; then file contents, as today.
- `server` (the hidden self-hosted server): stores trees like objects and
  `<id>.v2.json` like snapshots. No other change.
- GC and pruning: follow trees; never prune trees locally.
- `.dawgit/index.json` (local stat cache, 17.7 MB at 100,000 files and
  rewritten on every commit) becomes a compact binary file, read once per
  process. This is local only and can ship in 0.8.

## Expected results (100,000 files)

|                                | today   | format 2              |
|--------------------------------|---------|-----------------------|
| record per commit (10 changes) | 16.5 MB | ~30 trees, tens of KB |
| sharing a commit               | 16.5 MB | the same tens of KB   |
| 31 versions on disk            | 512 MB  | ~20 MB                |
| comparing two versions         | both whole records | changed folders only |

It also prepares partial download: a folder not downloaded is one tree hash
the version keeps from its parent.

## Limits

- A folder with very many files (say 20,000 textures in one folder) has one
  big tree, rewritten whenever any file in it changes (about 2 MB). If that
  turns out to matter, big trees can be split into fixed-size parts later
  without changing anything above them.
- Format-2 projects can't be opened by DAWGit 0.7 or older.
