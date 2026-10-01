# Command line tool

The desktop app covers everyday use, including creating a team on Cloudflare R2. The command line tool `dawgit.exe` is for a few advanced tasks. It is installed in the `bin` folder of the DAWGit install folder (`%LOCALAPPDATA%\Programs\DAWGit\bin`). Run `dawgit help` for the full list.

## Team storage (S3-compatible)

```
dawgit connection-code --endpoint URL --bucket NAME --access-key K --secret-key S [--prefix P] [--name TEAM]
```

The same as **Create a team** in the app, for scripts: checks that the key can read and write the bucket (conditional writes included) and prints the connection code members paste into DAWGit. `--name` names the team for everyone. The code contains the key: send it privately.

## Self-hosted team server (advanced)

Instead of storage, a team can run its own server on a computer or NAS that is on while the team works. The app does not offer this when creating a team; members of such a team join by pasting the server address where the app asks for a connection code, then entering the token.

```
dawgit serve [--data DIR] [--addr :7331] [--name TEAM]
```

It prints the address and the access token members need, for example:

```
DAWGit server 0.6.1 for team "Team on STUDIO-PC"
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

`dawgit agent` keeps running while you work and tells you when someone commits a new version on your branch. It never changes your files. The desktop app does the same in the background.

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

## Working with Live Sets directly

```
dawgit info <set.als>                          # tracks, devices, clips, automation, plugins, samples
dawgit diff <a.als> <b.als>                    # semantic diff
dawgit merge-sets <base> <ours> <theirs> -o out.als [--strategy fail|ours|theirs|both]
```
