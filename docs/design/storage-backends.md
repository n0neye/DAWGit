# Design: pluggable storage backends

Status: steps 1–5 of the implementation plan are done. The direct object storage backend passes the contract tests against an in-memory fake, Versity S3 Gateway v1.8.0 and Cloudflare R2 (including concurrent branch updates). Step 6 is open.

## Summary

Today a DAWGit client talks to exactly one kind of backend: the `dawgit serve` HTTP server. This document proposes a `Backend` interface in the client with three implementations sharing one storage layout:

| Mode | Backend | Runs where |
|---|---|---|
| **Server** (today) | HTTP API of `dawgit serve` | a team member's computer, a NAS, a VM |
| **Direct object storage** | clients read and write an S3-compatible bucket directly | any S3-compatible service, or a self-hosted S3-compatible server (e.g. on a NAS) |
| **Hosted service** (possible later) | HTTP API for metadata; file transfers go straight to object storage via presigned URLs | a managed deployment |

Because all three use the same object layout, a team can move between them by copying data.

## What the backend has to provide

Almost everything DAWGit stores is immutable and named by its content hash. Only two kinds of data change over time:

| Data | Mutability | Consistency needed |
|---|---|---|
| File contents (`objects/<sha256>`) | immutable | read-after-write |
| Version manifests (`snapshots/<id>.json`, id = sha256 of the bytes) | immutable | read-after-write |
| Project metadata (`project.json`) | rarely changes | last writer wins |
| **Branch heads** | changed by several people | **compare-and-swap** |
| Workspace states (unsaved work, soft locks) | each written by one workspace only | last writer wins |

Branch heads are the only data that needs an atomic update: a save moves a branch from the version the client last saw to its new version, and must fail if someone else moved it in between. The client then merges and retries (this loop exists today).

## The interface

The current `internal/remote.Client` becomes one implementation of:

```go
type Backend interface {
	// Projects
	Projects() ([]Project, error)
	PutProject(p Project) error

	// Branches: name -> version id. UpdateBranch is compare-and-swap:
	// old == "" creates, new == "" deletes, ErrConflict if the branch moved.
	Branches(pid string) (map[string]string, error)
	UpdateBranch(pid, name, old, new string) error

	// Versions (immutable, id = sha256 of the manifest bytes)
	MissingSnapshots(pid string, ids []string) ([]string, error)
	PutSnapshot(pid, id string, data []byte) error
	GetSnapshot(pid, id string) ([]byte, error)

	// File contents (immutable, named by sha256)
	MissingObjects(hashes []string) ([]string, error)
	PutObject(hash string, r io.Reader) error
	GetObject(hash string) (io.ReadCloser, error)

	// Workspace states (unsaved work, soft locks)
	PutWorkspace(pid, wsid string, state any) error
	Workspaces(pid string, out any) error
}
```

`internal/project` depends only on this interface. A backend is chosen from the connection settings stored in `.dawgit/config.json`.

## Storage layout

Shared by the object-store backend and (after a small migration) the server's on-disk storage:

```
<prefix>/objects/<ab>/<cdef…>                    file contents, sha256
<prefix>/projects/<pid>/project.json             {id, name}
<prefix>/projects/<pid>/snapshots/<id>.json      version manifests
<prefix>/projects/<pid>/branches/<name>          one object per branch; body = version id
<prefix>/projects/<pid>/workspaces/<wsid>.json   workspace states
```

Objects are shared by all projects under a prefix, so a sample used in several songs is stored once. The server currently keeps all branches of a project in a single `branches.json`; moving to one object per branch avoids contention between branches and matches the object-store layout.

## Rules that keep the data consistent

These rules are enforced by the server today. With direct object storage there is no server, so every client must follow them:

1. **Write order**: file contents, then the manifest, then the branch. A branch never points to a version whose manifest or files are missing.
2. **Verify on read**: every downloaded object and manifest is checked against its hash. A corrupt or truncated object is treated as missing.
3. **Immutable writes are idempotent**: uploading an object that already exists is harmless, so interrupted uploads are simply retried.
4. **Only branch updates need compare-and-swap.** Everything else is either immutable or has a single writer.

## Direct object storage backend

### Branch compare-and-swap

S3-compatible services support conditional writes:

| Operation | Request |
|---|---|
| create branch | `PUT branches/<name>` with `If-None-Match: *` |
| move branch | `GET` (remember `ETag`, check body == old), then `PUT` with `If-Match: <etag>` |
| delete branch | `DELETE` with `If-Match: <etag>` where supported |

`412 Precondition Failed` maps to `ErrConflict`. Support and consistency guarantees differ between services, so each supported service must pass the backend contract tests (below) before it is listed as supported.

### Existence checks

`MissingObjects` / `MissingSnapshots` use `HEAD` requests with bounded parallelism. Clients remember what they have already uploaded to a given backend to avoid repeated checks.

### Polling

There is no push channel, so the agent polls. To keep request counts low:

- read branch heads and known workspace states with `GET` on fixed keys rather than listing prefixes;
- list the workspaces prefix only occasionally to discover new members;
- use a longer interval than the server backend (e.g. 15–30 s instead of 5 s), and back off while nothing changes.

Soft locks and new-version notices therefore arrive with a delay of up to one interval.

### Credentials and access

Each member gets their own access key limited to the team's bucket (or prefix), so one member can be removed without affecting the others. Every key can read and write everything in the bucket; this mode assumes a small group that trusts each other. Services that offer object locking or version retention can be used to protect history against accidental deletion.

To avoid asking musicians for an endpoint, bucket and two keys, the desktop app accepts a single **connection code**: an encoded bundle of endpoint, bucket, prefix and credentials, created by whoever sets up the bucket.

### Garbage collection

Deleting data is optional and never automatic. A later `dawgit gc` can remove objects that are not reachable from any branch, version or workspace backup, and that are older than a grace period, so that uploads in progress are never collected.

## Two layers: Bucket and BucketBackend (0.13)

Storage teams are built in two layers, so a new way to reach storage gets
every feature at once:

- **`Bucket`** (`internal/remote/bucket.go`): keys and bytes only. Get (with
  an etag), Open, Exists, Put (optionally checked against a SHA-256, and
  conditional: only if absent, or only if the etag still holds), Delete,
  List (in key order, a page at a time, from a key) and Folders. It knows
  nothing about DAWGit. Implementations: `s3Bucket` (signed S3 requests,
  multipart for big objects, retries for small requests); later the hosted
  service's brokered access (presigned URLs, branch moves through its API).
- **`BucketBackend`** (`internal/remote/s3.go`, `gc.go`, `keys.go`):
  everything a team stores, written once on top of a Bucket: projects,
  versions, branches (compare-and-swap through conditional writes), members,
  workspaces, compressed and chunked contents, setups, backup records,
  cleanup with leases. Backups and restores (`internal/backup`) read and
  write through it too.

Every branch move is also recorded (`projects/<pid>/branchlog/<time>-<random>.json`:
branch, from, to, member, time), one new key per move, after the move
succeeded: storage itself keeps only where branches are now.

The team server (`dawgit serve`, `remote.Client`) implements `Backend`
directly and is frozen: it doesn't get the features above.

## Hosted service mode (possible later)

A hosted deployment splits the work:

- **Metadata API**: the same operations as the server's HTTP API, plus accounts, teams and permissions. It validates writes (the checks the server performs today) and can push branch and workspace changes over WebSocket instead of polling.
- **File transfers**: `PutObject` / `GetObject` return a redirect to a short-lived presigned URL, so file contents go directly between the client and object storage.

The client's HTTP backend already covers this mode if it follows redirects for object transfers and accepts an optional push channel. The self-hosted server can keep serving file contents itself.

Client-side encryption of file contents is compatible with this design because all diffing and merging happens in the client: the backend only ever needs hashes and bytes. Deduplication would then use a keyed hash per team.

## Migration between modes

With a shared layout, moving a team is a copy: objects and manifests first, then workspaces, then branches (following the write-order rule). A `dawgit migrate <from> <to>` command can do this incrementally and verify hashes.

## Implementation plan

1. Extract the `Backend` interface; make `remote.Client` implement it; `internal/project` uses the interface. No behaviour change.
2. Backend contract tests: one suite (branch CAS races, idempotent uploads, missing checks, workspace round trip) run against every implementation. Against a real service: `DAWGIT_TEST_STORAGE=<connection code> go test ./internal/remote -run Live -v`. The branch race test also catches services that accept conditional headers without enforcing them.
3. Server storage: one object per branch, with a one-time migration of `branches.json`.
4. S3-compatible backend (a minimal signed-request client), tested against a self-hosted S3-compatible server and at least one hosted service.
5. Connection codes in the desktop app and CLI; slower polling for this backend.
6. `dawgit migrate`, then `dawgit gc`.
