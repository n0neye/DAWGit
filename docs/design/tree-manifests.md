# Tree manifests (version format 2)

Status: built (0.8). Measured results at the end.

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
  "which are missing" check and parallel uploads, after the files they list
  and before the version record that names them.
- **This computer:** `.dawgit/trees/<ab>/<rest>`, apart from file contents.
  Trees are never pruned: they are the history. A downloaded version's
  trees are stored before its record, so a version stored here can always
  be read.
- **Memory:** parsed trees are cached by hash (shared by all projects in
  the process). Versions share nearly all their trees, so reading the next
  version of a project reads only the trees that changed.

## Old clients

No client before 0.8 checks a record's `version`. A format-2 record's
`files` is the number of files, not a list, so an older DAWGit fails to read
the record (and stops, changing nothing) instead of taking the version for
an empty project. From 0.8 on, clients refuse records of a newer format
than they know ("update DAWGit").

## Rollout

None: DAWGit is in testing, used by a couple of people on test projects.
From 0.8 every new version is format 2; format-1 versions stay readable, and
a format-2 version may have format-1 parents. Old versions are never
rewritten. Everyone on a team needs 0.8 once someone commits with it.

## What changed in the code

- `manifest`: format-2 records (`Tree`, `FileCount`, `TotalSize`); trees
  (`EncodeTree`, `ParseTree`, `BuildTrees`, `Flatten`). In memory a version
  still has the flat list of files, filled from its trees on load, so merge,
  checkout and status didn't change.
- `project`: commits write the trees of changed folders (a parent's trees
  are known stored); sync uploads and downloads trees; history and ancestry
  read records only; cleanup (GC) reads each folder's tree once across all
  versions.
- `server` (the hidden self-hosted server): checks a record's top tree was
  uploaded, as it checks files.
- `.dawgit/index.json` (the local stat cache, 17.7 MB at 100,000 files)
  became `index.bin`, a compact binary file; the old file is read once and
  replaced.

## Results (100,000 files)

|                                | format 1 | format 2             |
|--------------------------------|----------|----------------------|
| record per commit (10 changes) | 16.5 MB  | 307 B + 21 trees, 147 KB |
| all trees of the project       | -        | 1,024 trees, 9 MB    |
| commit, 10 files changed       | 0.43 s   | 0.35 s               |
| reading a version              | ~0.2 s (16.5 MB JSON) | 0.09 s (cached trees), 0.14 s cold |

The first format-2 commit of a project writes all its trees (about 1.5 s
here); later ones write only what changed.

It also prepares partial download: a folder not downloaded is one tree hash
the version keeps from its parent.

## Limits

- A folder with very many files (say 20,000 textures in one folder) has one
  big tree, rewritten whenever any file in it changes (about 2 MB). If that
  turns out to matter, big trees can be split into fixed-size parts later
  without changing anything above them.
- Format-2 versions can't be read by DAWGit 0.7 or older.
- Trees no version uses any more (after a branch is deleted) stay on disk;
  they are small.
