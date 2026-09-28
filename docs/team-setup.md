# Setting up DAWGit for your team (Windows)

DAWGit keeps the version history of your Ableton Live projects and lets 2–3 people work on the same song. One computer in the team (or a NAS) runs the **team server**; everyone installs the **DAWGit app**.

## 1. Install

Run `DAWGit-<version>-setup.exe` on every computer. No administrator rights are needed. Leave **Start with Windows** checked: DAWGit then waits in the system tray, backs up your work in progress, shows what teammates are editing and tells you about new versions. It never changes your project files on its own.

Requires Windows 10 (21H2 or later) or Windows 11.

## 2. Start the team server (one person)

Start menu → **DAWGit → DAWGit Team Server**. A window opens and shows:

```
DAWGit server
  data:  C:\Users\you\DAWGit Server
  token: 3f9c…

team members connect with:
  dawgit remote http://192.168.0.11:7331 --token 3f9c…
```

Share the **address** (`http://192.168.0.11:7331`) and the **token** with your team. Keep the window open while you work together (minimise it). All server data lives in `DAWGit Server` in your user folder: back that folder up.

Teammates outside your home/studio network need a way to reach this computer, e.g. a VPN such as Tailscale, or port forwarding of port 7331.

## 3. Share a project (the person who has it)

1. Open DAWGit → **+ Add project folder** → choose the Ableton project folder (the one with the `.als` file and `Ableton Project Info`).
2. Click **Connect to team server…**, enter the address and token.
3. Write what the song is at and click **Save version & share**.

## 4. Join a project (everyone else)

DAWGit → **Join from team server** → enter the address and token → **Find projects** → pick the song and a folder to download it into → **Join**. Samples that lived outside the project on the other computer are downloaded too, and the set is pointed at them.

## Everyday use

- Work in Live as usual and press **Ctrl+S**. Your changes appear under **Changes**, track by track.
- When you reach a point worth sharing, describe it and click **Save version & share**. If a teammate saved in the meantime, their changes are merged in first; if you both changed the same track, DAWGit asks which to keep (yours, theirs, or both as two tracks).
- When a teammate saves, DAWGit shows it. Click **Preview** to see what changed, **Get updates** to take it. Close the set in Live first: DAWGit changes the files on disk and Live would overwrite them.
- A yellow banner means you and a teammate are editing the same track right now: talk before you both save.

## Uninstall

Settings → Apps → DAWGit → Uninstall. Your projects, their `.dawgit` history folders and the server data are kept.
