# Command line tool

The desktop app covers everyday use. The command line tool `dawgit.exe` is for running a team server, setting up team storage, and a few advanced tasks. It is installed in the `bin` folder of the DAWGit install folder (`%LOCALAPPDATA%\Programs\DAWGit\bin`). Run `dawgit help` for the full list.

## Team server

```
dawgit serve [--data DIR] [--addr :7331] [--name TEAM]
```

Runs the team server. It prints the address and the access token members need. Data is kept as plain files in `--data`: back that folder up. The Start menu shortcut **DAWGit Team Server** runs this with `--data "%USERPROFILE%\DAWGit Server"`.

## Team storage (S3-compatible)

```
dawgit connection-code --endpoint URL --bucket NAME --access-key K --secret-key S [--prefix P] [--name TEAM]
```

Checks that the key can use the bucket and prints a connection code that members paste into DAWGit instead of a server address. `--name` names the team for everyone. The code contains the key: send it privately.

## Everyday commands (inside an Ableton project folder)

```
dawgit init [--author NAME]          # start tracking this project
dawgit remote <address> --token T    # connect it to a team server (or: dawgit remote <connection code>)
dawgit save -m "added drums"         # commit a version and share it (merges the team's versions first)
dawgit update [--preview]            # get the team's latest versions (or just look)
dawgit status                        # what changed, who is editing what
dawgit log                           # versions
dawgit clone <address> "Song" [folder] --token T   # download a team project
dawgit teams                         # teams this computer is connected to
```

`dawgit agent` keeps running while you work: it backs up unsaved work to the team, shows which tracks teammates are editing, and tells you when someone saves a new version. It never changes your files. The desktop app does the same in the background.

`save` and `update` merge Live Sets track by track. When you and a teammate changed the same track (or the same sample file) they stop and ask for `--strategy ours|theirs|both`. They refuse to rewrite sets while Ableton Live is running if the team's changes must be merged in.

## Branches (advanced)

```
dawgit branch                    # list branches and their latest versions
dawgit branch new yi-ideas       # start a branch from your current version
dawgit switch main
dawgit merge yi-ideas --preview  # what would come in and what conflicts
dawgit merge yi-ideas            # merge into your branch and share
```

## Local versions and restoring

```
dawgit snapshot -m "message"            # commit a version on this computer only
dawgit checkout <id|HEAD~N> [--force]   # restore a version and relink samples for this computer
```

Versions store file contents by SHA-256 in `.dawgit/objects` inside the project (deduplicated). Samples referenced from outside the project are stored too and, on a computer that lacks them, placed under `.dawgit/external/` with the set's sample paths rewritten. Samples from Live packs are only recorded by pack name. `Backup/` and `*.asd` are ignored.

## Working with Live Sets directly

```
dawgit info <set.als>                          # tracks, devices, clips, automation, plugins, samples
dawgit diff <a.als> <b.als>                    # semantic diff
dawgit merge-sets <base> <ours> <theirs> -o out.als [--strategy fail|ours|theirs|both]
```
