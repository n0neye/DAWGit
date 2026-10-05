# Command line tool

The desktop app covers everyday use, including creating a team on Cloudflare R2. The command line tool `dawgit.exe` is for a few advanced tasks, scripts and AI agents. It is installed in the `bin` folder of the DAWGit install folder (`%LOCALAPPDATA%\Programs\DAWGit\bin`), and the installer puts that folder on your PATH: open a new terminal and type `dawgit`. Run `dawgit help` for the full list.

**Scripts and AI agents:** `status`, `log`, `save`, `update`, `merge` and `version` take `--json` (one JSON object on stdout, errors with fixed codes), and no command ever waits for an answer. See [DAWGit for AI agents](agents.md), also printed by `dawgit help agents`.

## Team storage (S3-compatible)

```
dawgit connection-code --endpoint URL --bucket NAME --access-key K --secret-key S [--prefix P] [--name TEAM]
```

The same as **Create a team** in the app, for scripts: checks that the key can read and write the bucket (conditional writes included) and prints the connection code members paste into DAWGit. `--name` names the team for everyone. The code contains the key: send it privately.

## Self-hosted team server (advanced)

> The team server keeps working but gets no new features (storage teams do: compression, big-file pieces, backups, setup sharing). For a team of your own, prefer team storage.

Instead of storage, a team can run its own server on a computer or NAS that is on while the team works. The app does not offer this when creating a team; members of such a team join by pasting the server address where the app asks for a connection code, then entering the token.

```
dawgit serve [--data DIR] [--addr :7331] [--name TEAM]
```

It prints the address and the access token members need, for example:

```
DAWGit server 0.13.1 for team "Team on STUDIO-PC"
  data:  C:\Users\you\DAWGit Server
  token: 3f9c…

team members connect with:
  dawgit remote http://192.168.0.11:7331 --token 3f9c…
```

Data is kept as plain files in `--data`: back that folder up. Deleted songs are moved to its `trash` folder. Teammates outside your network need a way to reach the computer, e.g. a VPN such as Tailscale, or port forwarding of port 7331.

## Everyday commands (inside an Ableton project folder)

```
dawgit init [--author NAME]          # start tracking this project
dawgit remote <address> --token T    # connect it to a team server (or: dawgit remote <connection code>)
dawgit save -m "added drums"         # commit a version and share it (merges the team's versions first)
dawgit update [--preview]            # get the team's latest versions (or just look)
dawgit status                        # what changed since your last version
dawgit log                           # versions
dawgit clone <address> "Song" [folder] --token T   # download a team project
dawgit teams                         # teams this computer is connected to
```

`dawgit watch` keeps running while you work and tells you when someone commits a new version on your branch. It never changes your files. The desktop app does the same in the background.

`save` and `update` merge Live Sets track by track. When you and a teammate changed the same track (or the same sample file) they stop and ask for `--strategy ours|theirs|both`. They refuse to rewrite sets while Ableton Live is running if the team's changes must be merged in.

## Branches (advanced)

```
dawgit branch                    # list branches and their latest versions
dawgit branch new yi-ideas       # start a branch from your current version
dawgit switch main
dawgit merge yi-ideas --preview  # what would come in and what conflicts
dawgit merge yi-ideas            # merge into your branch and share
```

## Older versions

```
dawgit checkout <id|HEAD~N> [--force]   # put the project in the state of a version (samples relinked)
dawgit checkout latest                  # back to the latest version
dawgit export <id|HEAD~N> <folder>      # write a version as a separate project folder
dawgit snapshot -m "message"            # commit a version on this computer only
```

On an older version, newer versions are kept. Committing, getting updates and merging wait until you go back to the latest version; to continue from the older one, start a branch there (`dawgit branch new NAME`). An exported copy has no DAWGit history; samples from outside the project are copied into its `Samples/Imported`.

Versions store file contents by SHA-256 in `.dawgit/objects` inside the project (deduplicated). Samples referenced from outside the project are stored too and, on a computer that lacks them, placed under `.dawgit/external/` with the set's sample paths rewritten. Samples from Live packs are only recorded by pack name. `Backup/` and `*.asd` are ignored.

## Checking the history

```
dawgit verify            # read every version and stored file again, report damage
dawgit verify --repair   # bring back what can be: from the project folder or the team's storage
```

The app does the same from a project's ⋯ menu in the sidebar (**Check project…**). A file damaged here (a failing disk, say) comes back from a file in the project folder with the same content, or from the team's storage; what has no copy left anywhere is listed with the version it's in.

## Backing up the team

```
dawgit backup run                   # back up where the app backs this team up (a folder or a bucket)
dawgit backup run D:\Backups\Band   # ...or to any folder: empty, or this team's earlier backup
dawgit backup status                # this computer's backup, and who else backs up the team
dawgit backup restore --preview     # what restoring would bring back (and the runs to pick from)
dawgit backup restore [folder] [--run 20261004-153000]
```

`--team NAME` picks the team; otherwise it is the team of the project you're in, or the current one. A backup holds every project of the team with all its versions, as the team's storage keeps them. Runs only copy what is new and never delete: what the team deleted stays in the backup. Each run records where every branch was (`runs/<time>.json`).

Or in the app: the team's settings (⚙) → **Backup**: a folder (a drive, a NAS), or **Another bucket…** (any S3-compatible storage, with its own keys; not the team's own storage). The app backs up once a day while it is open. Works on teams that use S3 or R2 storage. `restore` copies back only what the team's storage lacks (deleted projects, lost files, or everything into a new, empty bucket) and never changes or deletes anything there; `--run` restores as of an earlier backup run (projects made later stay out; branches the team still has stay as they are). See [design/backup.md](design/backup.md).

## Cleaning up the team's storage

```
dawgit storage-cleanup            # files in the team's storage no version of any project uses
dawgit storage-cleanup --delete   # delete those that are due
```

Or in the app: the team's settings (⚙) → **Storage cleanup**. Deleted projects and uploads that stopped leave files behind. A file is deleted only once a cleanup has found it unused a day before and it is at least a week old; files a teammate's share in progress relies on are never deleted. Works on teams that use S3 or R2 storage.

## Working with Live Sets directly

```
dawgit info <set.als>                          # tracks, devices, clips, automation, plugins, samples
dawgit diff <a.als> <b.als>                    # semantic diff
dawgit merge-sets <base> <ours> <theirs> -o out.als [--strategy fail|ours|theirs|both]
```
