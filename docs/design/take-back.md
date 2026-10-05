# Undo commit

The app's **Undo commit** does one of two things:

- **Take back** (`Repo.TakeBack`): the latest version leaves the history.
  The branch goes back to the version before it, and the version's changes
  come back as uncommitted changes, to fix and commit again. The branch log
  still records the move.
- **Revert** (`Repo.UndoCommit`): a new version takes back what a version
  changed and keeps everything that came after it. This is a 3-way merge
  with the version as base, the latest as ours and its parent as theirs.

Take back is used when it is safe; otherwise the app reverts.

## When a version can be taken back

All of these must hold (`Repo.PlanTakeBack`):

- It is the latest version of the branch, here and on the team.
- It is your own. Versions with no author id, or a member who hasn't
  chosen a name, don't count as yours.
- It is not a merge or the first version.
- No other branch has it.
- No other copy of the project has taken it in.

A version that was never shared is only moved here.

## Who has it: workspace records

Each copy of a project keeps a record in the team's storage, under
`projects/<pid>/workspaces/<id>.json`, with `{id, member, branch, has}`.
`has` is the team's latest version that copy took in. The copy writes it
before its files change, when it updates, catches up or shares
(`noteTeamHead`).

A copy also keeps the same version in `.dawgit/team-seen.json`, per branch.

Old DAWGits don't write these records, and could share a taken-back
version again. That makes taking back a team feature (`take-back`). The
first time someone takes back a version, the app turns the feature on, and
DAWGits without it stop before working with the team.

## A copy that had it anyway

A copy can still end up with a taken-back version: it may have taken the
version in at the same moment, or before its record went up. That copy
notices because the team's branch no longer contains the version it saw
last (`TakenBackFrom`).

A copy from before the records has no version seen. For it, the taken-back
versions are those it has by other members that no branch of the team has.

On Update or Share, `dropTakenBack` cleans the copy up:

- The copy goes to the team's latest version.
- Its own unshared versions are replayed after that version.
- Its uncommitted changes are merged in and stay uncommitted.

The app shows who took back what, the same way it shows incoming versions,
and Live is checked first because files change.
