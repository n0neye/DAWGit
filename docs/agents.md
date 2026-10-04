# DAWGit for AI agents

How an AI agent (Claude Code, Codex, Cursor…) uses DAWGit in a user's
project. Also in the CLI: `dawgit help agents` (this page, for the installed
version) and `dawgit help agents --snippet` (a few lines for a project's
`AGENTS.md` or `CLAUDE.md`).

DAWGit is version control for creative projects: Ableton Live sets, Unity
and Unreal projects. A project folder with a `.dawgit` folder is tracked.
**`save` makes a version and shares it with the team in one step**; there
is no separate commit and push.

## Rules

- **Use `--json`** with `status`, `log`, `save`, `update`, `merge` and `backup`, and
  read the result, not the text.
- **Commands never wait for an answer.** When one is needed they stop with
  an error code (below). Never pipe answers into DAWGit.
- **Don't use `--force` on your own.** It makes DAWGit rewrite sets while
  Ableton Live may have them open, or discard changes. Ask the user.
- **Never print connection codes** (`dawgit connection-code`, `dawgit
  remote <code>`, `dawgit clone <code>`): they contain the team's storage
  keys. Don't put them in files, logs or messages.
- **Don't edit `.dawgit/`.** It is DAWGit's own data. `.dawgit.yaml` (the
  project's rules: what is tracked) is fine to read and edit; `dawgit profile
  check` validates it.
- Version ids are long hex strings; any unique prefix (10 characters, as
  the text output shows) works where a command takes one.

## Everyday flow

```sh
dawgit status --json            # what changed; is the team ahead?
dawgit update --preview --json  # what the team saved: versions, changes, conflicts
dawgit save -m "Brighter mix on the chorus" --json
```

`save` merges what teammates saved in the meantime before sharing. When the
same track (or file) changed on both sides it stops with `merge_conflict`:

```sh
dawgit update --preview --json  # result.conflicts: file, unit (track), description
# decide with the user, then:
dawgit save -m "..." --strategy theirs --json
```

Strategies: `ours` (keep this computer's), `theirs` (keep the team's),
`both` (keep both: the track twice, or the file under a new name, when
`can_keep_both`). A strategy applies to every conflict of that run.

To only get the team's versions: `dawgit update --json`. Changes not saved
yet stay as they are (still not saved) and the team's versions are merged
into the files; when you and a teammate changed the same file or track it
stops with `merge_conflict` first (`--strategy` decides). `result.kept_work`
is true when uncommitted changes were kept.

## Output

Every `--json` command prints one object on stdout:

```json
{"schema": 1, "ok": true, "command": "status", "result": { ... }}
{"schema": 1, "ok": false, "command": "save",
 "error": {"code": "merge_conflict", "message": "...", "hint": "...", "exit": 3, "conflicts": [ ... ]}}
```

Warnings (e.g. "wrote .dawgit.yaml") go to stderr. `schema` changes only
when a field changes meaning; new fields may appear any time.

`status` result:

| field | |
|---|---|
| `project`, `branch` | names |
| `version` | the version the files are on (`""` before the first) |
| `on_older_version`, `latest` | an older version is checked out; `dawgit checkout latest` goes back |
| `team` | `null` without a team, else `reachable`, `incoming` (others saved versions: update), `error` |
| `changes` | `path`, `status` (`added`, `modified`, `deleted`, `renamed` with `from`, `untracked`: still on disk but the rules leave it out), `set_changes` (for Live sets: tracks, devices, clips changed, one line each), `weight` (for Live sets, the biggest kind of change: `noise` a plugin re-saving its own state, `tidy` names/colors/order/groups, `mix`, `sound` devices, `arrangement` clips/notes/tracks/tempo) |
| `suggestions` | tool projects found in folders the rules don't cover: `dawgit profile preset <folder> <preset>` |

`log` (`-n N` for the newest N): `versions`, newest first: `id`, `time`
(RFC 3339), `author`, `message`, `parents`, `branches` (team branches at
that version).

`save`, `update` and `merge`: `action` (`published`, `local`,
`nothing-changed`, `up-to-date`, `ahead`, `fast-forward`, `merged`),
`saved` (the version saved, or null), `from`, `to`, `merged` (what a merge
took from each side), `relinked` (sample paths rewritten for this computer),
`reopen_sets` (sets changed under Live: the user must reopen them).

`update --preview` and `merge <branch> --preview`: `action`, `versions`
(incoming), `changes`, `conflicts` (`key`, `file`, `unit`, `description`,
`can_keep_both`).

`backup run [folder]` (back up the whole team's storage into a folder; only
adds): `team`, `folder`, `run` (its record: `<folder>/runs/<run>.json`),
`keys`, `copied`, `copied_bytes`, `total_bytes`. Without a folder it uses
the one chosen in the app. The first run can copy many gigabytes.

`backup status`: `team`, `supported` (storage teams only), `folder` (this
computer's, `""` if none), `last_success`, `last_attempt`, `error`,
`failing`, `members` (who backs the team up: `name`, `last_success`,
`failing`), `covered` (one of them did in the last 7 days).

## Errors

| exit | code | what to do |
|---|---|---|
| 2 | `usage` | fix the command (`dawgit help`) |
| 3 | `merge_conflict` | show `conflicts` to the user; run again with `--strategy ours\|theirs\|both` |
| 4 | `set_open_in_live` | ask the user to save and close `set` in Live, then run again |
| 5 | `team_unreachable` | network or storage down: try again later |
| 5 | `project_busy` | the DAWGit app or another command is using the project: try again in a moment |
| 6 | `not_a_project` | not in a tracked folder: `dawgit init`, or `cd` into the project |
| 6 | `newer_version_needed` | the team uses a newer DAWGit: the user must update |
| 6 | `server_out_of_date` | the team's server needs updating |
| 1 | `unsaved_changes` | save first (`dawgit save -m ...`); `update` keeps them unless you also have versions not shared |
| 1 | `on_older_version` | an older version is checked out: `dawgit checkout latest` |
| 1 | `unshared_versions` | `dawgit save -m ...` shares them |
| 1 | `not_connected` | the project is not in a team (`save` still saves locally) |
| 1 | `files_not_here` | an old version's files are only in the team's storage |
| 1 | `backup_folder_missing` | the backup folder isn't there: ask the user to connect the drive |
| 1 | `backup_folder_not_empty` | `backup run` needs an empty folder or this team's backup |
| 1 | `backup_folder_taken` | the folder holds another team's backup |
| 1 | `error` | anything else: show `message` to the user |

Codes are stable; messages are for people and may change.

## Good to know

- DAWGit tracks whole projects, including big binary files (audio, Unreal
  levels). Saving can upload a lot the first time; later only what changed.
- Live sets (`.als`) merge track by track; other files merge only when one
  side changed them.
- `dawgit status` shows what changed inside a modified set
  (`set_changes`); `dawgit diff a.als b.als` compares two sets.
- `dawgit watch` keeps running and reports teammates' new versions; it is
  for people (no `--json`).
- The CLI is `dawgit` on PATH after installing the app (a new terminal sees
  it); otherwise `%LOCALAPPDATA%\Programs\DAWGit\bin\dawgit.exe` (or
  `DAWGit Pro\bin`). `where dawgit` shows which one runs.
