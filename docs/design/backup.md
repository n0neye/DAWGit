# Team backup

Status: built (0.12; another bucket 0.12.2; restore 0.12.3; from any backup bucket 0.13.12). Teams on S3-compatible storage (R2, S3) only; teams
on a self-hosted server back up the server's data folder instead.

## Why

A team's work lives in one bucket. A deleted bucket, a lost key, a
mistaken cleanup or a provider problem would take every project's history
with it, and the copies on members' computers hold only what each of them
downloaded. A second copy on a drive or NAS one member owns covers that.

## What a backup is

A folder (a drive, a NAS), or a folder of another bucket in any
S3-compatible storage, holding the team's storage as it is: every key at the same path
(`objects/ab/…`, `chunked/…`, `projects/<id>/snapshots/…`, `branches/…`,
`members/…`, `team.json`, `setups/…`). Nothing is converted, so whatever
reads storage can read a backup, and restoring is copying it back.

Left out: `gc/` (cleanup's bookkeeping, meaningless elsewhere) and
`backups/` (members' backup records).

Added next to the keys:

- `dawgit-backup.json`: whose backup the folder is (the team's id). A run
  needs it to be there, so an unplugged drive fails instead of filling the
  empty folder Windows shows in its place, and two teams never share a
  folder.
- `runs/<time>.json`: where every branch of every project was after that
  run. Branches move and backups keep only the latest branch files, but
  every version they ever pointed at stays, so a run record is enough to go
  back to any day.
- `README.txt` for whoever finds the folder.

## Add-only, incremental

Contents (`objects/`, `chunked/`, version records) are named by their hash
and never change once written: copied once, when missing (or a different
size: a copy cut short). The few keys that do change (branches, members,
the team's name, workspaces, setups) are copied again when their size or
time differs.

Nothing is ever deleted from a backup: a project the team deleted, or files
storage cleanup removed, stay. That is the point (a mistaken delete is one
of the things a backup is for); the cost is that a backup only grows.

Order matters for a consistent backup while the team keeps working: the
team writes contents before it moves a branch, so a run copies the changing
keys first and then lists storage again for the contents. Every version a
copied branch names is then in the backup by the end of the run.

Each copy is read for exactly the size the listing gave (shorter or longer
fails), eight at a time. In a folder it goes to `.tmp/` first and is
renamed into place; in a bucket a failed upload leaves nothing. A run that stops (the app closed, the drive
pulled) leaves no half files; the next run picks up where it left off.

## Into another bucket

A bucket takes the same keys at the same paths. The differences:

- A run lists the backup once at the start (cheaper than asking key by key)
  and compares against that list.
- A bucket can't keep the team's times on its copies; a changing key is
  copied again when the team's copy is newer than the backup's (the time the
  backup wrote it), which works for folders too.
- It must not be the team's own storage, or a folder in it or above it: a
  backup there would copy itself, and be lost with the team's storage.
- Its keys are checked (list, write, delete; no conditional writes needed)
  and kept in teams.json sealed like the team's. Keys of its own are best: a
  leaked or lost team key then can't reach the backup.

## Schedule and who backs up

The app backs up once a day while it runs (checked every 15 minutes,
starting two minutes after launch, so a computer that was off catches up);
after a failure it tries again hourly. `dawgit backup run` backs up on
demand, e.g. from a scheduled task on a NAS.

Each member who backs up notes it in the team's storage,
`backups/<member>.json`: kind, last success, last attempt, failing. No
paths. With that:

- Nobody is reminded to set one up while some member backed up in the last
  7 days. Otherwise the sidebar suggests it, at most weekly, and only once
  the team is settled on this computer: 3 days after joining it, or 3
  projects (someone trying DAWGit out isn't warned of what could go wrong;
  see [../ux-principles.md](../ux-principles.md)).
- Creating a team on storage offers it in one quiet line (**Set it up
  now…**), not as a step of its own.
- Team Settings shows this computer's backup at once and the other
  members' after; it leaves the section out while the team's storage can't
  be reached.
- A member's own failures are only shown after 3 days without a backup: a
  drive unplugged for a day is normal.

## Restoring

Back into the team's storage (Team Settings → Backup → Restore…, or
`dawgit backup restore`), from this computer's backup, any backup folder,
or any backup bucket other than the team's own storage (a teammate's,
say: its address and keys are given for the restore, read-only keys are
enough, and they aren't kept; the CLI takes them as a connection code):

- Only what the storage lacks is copied, and nothing is overwritten
  (records and branches go up with `If-None-Match: *`), so a restore can't
  undo a teammate's work. Branches the team still has stay where they are.
- Order: contents, then version records (`project.json` last, so a project
  shows up whole), branches last: a branch never names a version whose
  files aren't back yet.
- As of a run: projects made after it stay out, and missing branches come
  back where the run record has them. Contents are always welcome (they are
  addressed by hash).
- A preview (the plan) lists the projects coming back, with versions, the
  branches in projects the team still has, and the bytes to copy.
- A whole team lost: set up a new team on a new, empty bucket, then restore
  into it; the team's name stays the new one, members come back.

Not restored: `runs/`, `README.txt`, `dawgit-backup.json`, `gc/`,
`backups/`.

## Not yet

- Teams whose storage is a shared folder.
