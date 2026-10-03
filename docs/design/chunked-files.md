# Chunked big files

Status: proposed (after 0.9.6 compression). Not built.

## Why

A file is stored whole, named by the SHA-256 of its contents. Change one
actor in a 285 MB Unreal level and the whole level goes up again — since
0.9.6 compressed, which still leaves 40–60% of it for big `.umap` files: the
rest of a level is data Unreal already compressed (landscape, lighting).

Measured on a real Unreal project (IgnoranceOnline), with pairs of the same
level from Unreal's autosaves and copies (bytes to upload for the newer one,
as a share of its size):

| pair                                   | whole file, zstd (0.9.6) | chunks ~1 MB, zstd |
|----------------------------------------|--------------------------|--------------------|
| S1-v2, two saves the same day          | 59%                      | 0.2%               |
| S1-v2, two days apart                  | 59%                      | 0.2%               |
| S1, a week apart                       | 59%                      | 1.7%               |
| S4_2, two weeks apart                  | 47%                      | 0.2%               |
| S4_2, four months apart                | 47%                      | 0.2%               |
| S1 → S1-v2 (copied, then edited)       | 59%                      | 0.2%               |
| S4_All, a big edit (248 → 283 MB)      | 58%                      | 35% (64 KB: 24%)   |
| S4_1 → S4_All (another level)          | 58%                      | 55%                |

Chunks cut where the content says (content-defined chunking); fixed-size
chunks don't work here: an insert shifts everything after it (fixed 1 MB:
86–100% new). Chunking a 285 MB level takes ~0.6 s.

So: an everyday edit to a big level goes from ~170 MB to under 1 MB, up and
down (teammates download the same few chunks).

## The idea

Nothing about versions changes: trees still name each file by the SHA-256 of
its whole contents, with its size. What changes is what is stored under that
name for a big file: instead of its contents, a **chunk list** — the hashes of
the pieces, in order. The pieces are ordinary objects (compressed blobs) in
the same `objects/` folder, so identical pieces are stored once across files,
versions and projects.

```
tree:     f 9c1e…  285671001  DesertCity-S1-v2.umap
objects/9c/1e…  →  chunk list:  4a07… 1048576
                                e2b9… 893122
                                …            (~190 lines)
objects/4a/07…  →  zstd blob of those 1048576 bytes
```

A teammate's DAWGit that doesn't know chunk lists reads the header and stops
with "unknown mode … made by a newer DAWGit?" (0.9.6), never with wrong
contents.

## Format

### Chunk list (blob mode `c`)

`blob.Magic` + `'c'` + zstd of text:

```
dawgit-chunks 1 fastcdc-gear 262144 1048576 4194304
<chunk sha256> <size>
<chunk sha256> <size>
…
```

- Header: format, algorithm, min / average / max chunk size. Readers don't
  need the parameters (they just join the pieces); they are there so a later
  DAWGit can change them — the new pieces then simply don't match old ones.
- Joining the pieces must give the file: its size (from the tree) and its
  SHA-256 (the object's name) are checked on download, as today.
- A 285 MB file: ~190 lines, ~14 KB (hashes don't compress much).

### Chunks

Ordinary blobs (0.9.6): named by the SHA-256 of their own bytes, zstd when
that saves 10%, as they are otherwise.

### Chunking

Gear hash, normalized (FastCDC): no cut before 256 KB, harder to cut before
1 MB, easier after, always cut at 4 MB. The gear table is fixed (part of the
format: a different table means no piece matches an old one).

Averages measured above: 64 KB saves more on big edits (24% vs 35%) but
means 16× the objects and requests; 1 MB is as good for everyday edits.

### Which files

Files of at least **16 MB** (`chunk.MinFile`), in team storage (S3/R2) only.
Smaller files stay whole: few pieces would match, and every piece costs a
request. Unity scenes (~8 MB here) are below it and already compress to ~6%.

A DAWGit server keeps contents as they are (it checks each object's hash);
chunking there is later, with its own protocol.

## Sharing (upload)

For each file to upload of at least 16 MB:

1. Read it once: cut it into pieces, hashing each (and the whole, which is
   known already).
2. Ask storage which pieces are missing (`MissingObjects`; pieces of the
   file's previous version are usually there).
3. Upload the missing pieces (compressed, as any blob), in parallel, with
   retries.
4. Upload the chunk list under the file's hash — after its pieces, as
   versions are written after their files: a chunk list is never there
   without its pieces.

The lease a share writes (cleanup must not delete what a share relies on)
also lists the pieces. Progress counts the bytes actually sent.

Files already stored whole stay whole; nothing is re-uploaded.

## Downloading

`GetObject(file)`: whole contents as today, or a chunk list. For a list:

1. Pieces this computer already has are taken from here: when the previous
   version of the same file is in the project folder (or the store), its own
   chunk list (kept in `.dawgit/chunks/<hash>`, a few KB each, or worked out
   again by cutting the file) says which pieces it holds and where.
2. The rest are downloaded in parallel, each checked against its hash.
3. Pieces are written in order into the store; the whole file's SHA-256 is
   checked as today.

So a teammate pulling an everyday level edit downloads under 1 MB too.

## Cleanup (storage GC)

Today: every version's trees name the files in use; files nobody uses are
marked, then deleted a day later if still unused.

Now a file in use may be a chunk list, and its pieces are in use too. For
every file in use of at least 16 MB, cleanup reads the first bytes of its
object (a range request); for a chunk list, the whole list. Chunk lists never
change, so their pieces are cached between cleanups (`gc/chunks.json`).
Big files are few (hundreds), so this adds little.

The rest is unchanged: pieces uploaded in the last week are kept (a share
uploads pieces before its list), leases keep a running share's pieces, and
unused pieces are deleted only on a second cleanup.

## Checking (verify)

- Locally nothing changes (files are whole on this computer).
- `--team` checks, for big files, that the chunk list and its pieces exist;
  repair re-uploads missing pieces from a local copy.

## Compatibility

- Old blobs and whole files keep working; mixing is fine (a file's older
  versions whole, newer ones chunked).
- DAWGit 0.9.6 can't read chunk lists: the release that writes them must
  force updating (`-min`), as 0.9.6 did for compression.

## Costs

- First upload of a 285 MB level: ~190 objects instead of one (R2: ~$0.001).
  Compressing pieces one by one can be a little worse than the whole file
  (each piece starts afresh); to be measured.
- Every share of a changed big file asks about its ~190 pieces (one request
  each, in parallel, ~1 s). Later: skip pieces the previous version's list
  already has.
- Code: a new `internal/chunk` package; upload, download, cleanup and verify
  learn about chunk lists. Trees, versions, merges, the UI: unchanged.

## Plan

1. `internal/chunk`: cutting (fixed boundaries test: same bytes, same
   pieces, forever), chunk list format.
2. Upload: chunk lists for big files, leases with pieces, progress.
3. Download: pieces from local copies, the rest from storage.
4. Cleanup and `verify --team`.
5. Tests: round trips through the fake S3 (whole, chunked, mixed); an edit
   uploads only a few pieces; cleanup keeps and deletes pieces correctly;
   flaky storage; the shuffled-files test with big files.
6. E2E: the Unreal scenario (s10) edits a level and checks the bytes sent.

## Open questions

- 16 MB threshold: lower to 8 MB for Unity scenes? (They already compress
  to ~6%; pieces would save most of the rest for small edits.)
- Server-hosted teams: keep whole files for now?
