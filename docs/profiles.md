# Project rules (`.dawgit.yaml`)

Most projects need no setup: DAWGit recognizes an Ableton Live project and follows the built-in **ableton** preset. It tracks everything in the project folder except Live's `Backup` folder and its `.asd` analysis files.

To change what is tracked, add a `.dawgit.yaml` file to the project folder. In the app: **Rules** next to the team name at the top of a project → **Create .dawgit.yaml**. The file is committed with the project, so the whole team follows the same rules.

## Example

```yaml
requires: "0.7"          # the oldest DAWGit that understands this file
use:
  ./: ableton            # the project folder is an Ableton Live project
rules:                   # yours, on top of the preset; later rules win
  - ignore: "Exports/"   # leave every "Exports" folder out of versions
  - ignore: "*.tmp"
  - track: "*.asd"       # keep Live's analysis files after all
```

## Rules

Each rule is either `ignore:` (leave matching files out of versions) or `track:` (keep them, overriding an ignore from the preset or an earlier rule). When several rules match a file, the **last** one decides. Rules come before the preset.

Patterns work like `.gitignore` lines and ignore upper/lower case:

| Pattern | Matches |
| --- | --- |
| `*.tmp` | any file named `….tmp`, in any folder |
| `Exports/` | any folder named `Exports`, and everything in it |
| `/Notes.txt` | `Notes.txt` in the project folder only |
| `Samples/Recorded/*.wav` | `.wav` files in that folder (relative to the project folder) |
| `**/Bounces/` | a `Bounces` folder at any depth |

### The project's `.gitignore` files

With `gitignore: true` in `.dawgit.yaml` (or a preset that asks for it), DAWGit also follows the project's `.gitignore` files, as git reads them: each applies to its own folder, a deeper one wins over the one above, within a file the last matching line wins, and `!` takes a file back (but not out of a folder that is left out). Your `rules:` still come first.

```yaml
gitignore: true
rules:
  - track: "*.psd"       # a .gitignore leaves them out; keep them in versions
```

A few things are never tracked, whatever the rules say: DAWGit's own `.dawgit` folder, `.git` folders, and system files such as `desktop.ini` and `Thumbs.db`. `.dawgit.yaml` itself is always tracked.

## What happens when a rule changes

- Files that a new rule leaves out show up as **no longer tracked** (○) in the Changes tab. When you commit, DAWGit lists them and asks you to confirm. The next version doesn't have them.
- **Nobody's files are deleted.** When your teammates take in that version, the files stay on their computers, just untracked. Take them out of the rule and they are tracked again.

## Presets

`use:` says which preset applies to which folder. `./` means the project folder. With no `use:`, DAWGit detects the preset as it would without a `.dawgit.yaml`. `none` turns a folder's preset off, so only your rules apply there.

The built-in presets:

| Preset | For | Detected by | Leaves out |
| --- | --- | --- | --- |
| `ableton` | Ableton Live projects | an `Ableton Project Info` folder or a `.als` file | `/Backup/`, `*.asd` |

A preset can ask for the project's `.gitignore` files (`gitignore: true`), and has a `priority` for detection (0 by default): when several presets recognize a folder, the highest wins. So a folder with a `.gitignore` can be a code project (a low priority) unless it is also, say, a Unity project.

Detection also looks inside the project folder, up to three folders deep, for the projects of tools (presets of priority 0 and up, such as a Live or Unity project). Their preset applies in their folder: a Unity project in `Game/` leaves out `Game/Library/`, a Live project in `Music/Theme Project/` its own `Backup/`. Design files or a `.gitignore` in a folder don't make it a project of its own. With `use:`, only the folders it names get a preset.

A preset also tells DAWGit which built-in code handles which files, what to check before rewriting files, and how files are grouped and shown in the app. The handlers a preset can name:

| Handler | Kind | Does |
| --- | --- | --- |
| `ableton-set` | `merge:` | Compares and merges Live Sets track by track; with `samples: ableton`, collects and relinks their samples |
| `text` | `merge:` | Merges text files line by line when both sides changed them; if the same lines changed differently, you choose the whole file |
| `ableton-live` | `running:` | Doesn't rewrite a set while Live has it open |

Files without a merge handler are chosen whole (yours, theirs or both) when both sides changed them.

## Checking the rules

- The **Rules** dialog in the app shows the preset in use and any problem with `.dawgit.yaml`. While the file has a mistake (e.g. a misspelt field), DAWGit shows the problem and won't commit until it's fixed. Looking at changes still works.
- From the command line, inside the project folder:

  ```
  dawgit profile check                 # the rules in use, and whether they work
  dawgit profile explain Exports/mix.wav Backup
  ```

  `explain` says whether each file is tracked and which rule or preset decided it.

## Older DAWGit versions

`requires:` names the oldest DAWGit that understands the file (DAWGit fills it in when it creates the file). A DAWGit older than that refuses to commit to the project or take in its versions, and asks to be updated. This way two people never track different files.
