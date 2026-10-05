# Big files up early

Status: built (0.13.15). Storage teams.

A commit of a big file (a video, a long recording) used to do everything
at commit time: copy it into the history, then upload it. While the file was
being read, the project was on hold, and the whole upload came after. Now
big files go up in the background once they stop changing, so a commit of
them is done in a moment.

## Nothing is shared by it

Storage keeps contents named by their hash, and versions name the
contents they use. A content that went up early is in no version until a
commit names it, so teammates see nothing new. The commit then finds the
team already has it and neither copies nor uploads it.

## Which files

A file goes up early when it meets all of these
(`Repo.PreuploadCandidates`):

- It is a change (added or modified) and not a set.
- It is at least 50 MB (`PreuploadMin`).
- It hasn't changed for 2 minutes (`PreuploadStable`), so it isn't still
  being written.
- The team doesn't have it yet, as far as this copy knows.

The biggest go first. The app goes through them one at a time, and checks
every minute (`desktop/preupload.go`).

## Safely

- The file is first copied into `.dawgit/preupload/` and hashed on the way.
  If the hash isn't the one the file was found with, nothing goes up. What
  goes up is the copy, so it is exactly what its name says, whatever happens
  to the file meanwhile. The copy is removed once it's up.
- It goes up the same way a commit uploads (`uploadObjects`): as pieces for
  big files, compressed where that helps.
- The team's cleanup must not take it before a commit names it. A lease
  lists it (and its pieces) and is never released, so it lasts `leaseLife`
  (14 days). After that, an upload nobody committed is a leftover like any
  other.
- Here it is added to `remote-objects` (the team has it) and to
  `preuploaded` with the time. Local cleanup (`Repo.GC`) keeps such entries
  for 12 days, a little less than the lease. A commit after that copies the
  file into the history and uploads it again if the team no longer has it.
- It doesn't hold the project up. Finding, copying and uploading take no
  lock; only recording the upload takes the project's lock, for a moment.
  A project that is busy (an action holds it) is skipped until next time.

## In the app

- While a file goes up, a small moving cloud shows by the project in the
  sidebar and by the team's name on the project's page. Its tooltip names
  the file and how far it is.
- Team Settings → Your setup: **Upload big files in the background**, on
  unless turned off. It is set per computer (`teams.json`, `noPreupload`).

## Not yet

- Skipping metered connections, and limiting the speed.
