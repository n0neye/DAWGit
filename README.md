# DAWGit

**English** | [繁體中文](README-cht.md)

Version history and teamwork for Ableton Live projects. Commit versions of your songs, see what changed track by track, and work on the same song with your bandmates — DAWGit merges your changes at the track level.

> **Work in progress.** DAWGit is an early preview. Expect rough edges and changes (including to how data is stored) before 1.0. Keep your own backups of projects that matter. Windows only for now; tested with Ableton Live 12.

![DAWGit showing the tracks changed in a project, ready to commit a version](docs/images/screenshot.png)

## Features

- **Versions of the whole project** — Live Sets and samples, committed with a message. Unchanged files are stored once.
- **Changes by track** — after you press Ctrl+S in Live, DAWGit lists which tracks you added, removed or changed.
- **Track-level merging** — when two people edit the same song, their changes are combined track by track (tracks, their placement and order, sends) together with song-wide parts such as the main track, locators and scenes. Only when you both changed the same track does DAWGit ask which to keep: yours, theirs, or both side by side.
- **Samples go with the project** — samples inside the project folder, and samples from elsewhere on your disk, are stored with each version. On a teammate's computer the set is pointed at them automatically. Samples from Live packs are only recorded by name.
- **See who is editing what** — DAWGit shows which tracks your teammates are working on right now and warns you when you both edit the same one. Your unsaved work is backed up to the team.
- **Two ways to host a team**
  - a **team server** on any computer or NAS in your team (one program, no setup), or
  - **S3-compatible storage** such as Cloudflare R2 — nothing needs to stay switched on.
- **Or keep it local** — use DAWGit on your own, with versions kept on your computer only.
- **Nothing changes behind your back** — DAWGit never changes your project files on its own, only when you take in your team's changes, and it asks you to close Live before it rewrites a set.
- **Go back to any version** — put the project in the state of an older version from the History tab, then go back to the latest or start a branch from there. Or export a version as a separate project folder to open next to the current one.
- **Branches** for teams that want to try ideas separately and merge them later.

## Getting started

### Install

1. Download `DAWGit-<version>-setup.exe` from the [Releases](../../releases) page.
2. Run it. No administrator rights needed. The installer is not code-signed yet, so Windows may show "Windows protected your PC": click **More info → Run anyway**.
3. Leave **Start with Windows** checked: DAWGit then waits in the system tray, backs up your work in progress and tells you about new versions.

Requires Windows 10 (21H2 or later) or Windows 11.

### Use it on your own

1. Open DAWGit and choose **Just keep versions on this computer**.
2. Pick an Ableton project folder (the one with the `.als` file and `Ableton Project Info`).
3. Work in Live as usual and press **Ctrl+S**. Your changes appear in DAWGit.
4. Describe what you did and click **Commit version**.

You can share the project with a team later with **Share with a team…**.

### Join a team

Ask whoever set up your team for the **server address and token**, or for your **connection code**.

1. Open DAWGit, paste the address and token (or the code) and click **Connect**.
2. Enter your name — it appears next to the versions you commit.
3. Download the songs you work on, or add your own project with **+ Add a project**.

Everyday use:

- Work in Live and press **Ctrl+S**. Your changes appear under **Changes**, track by track.
- When you reach a point worth sharing, describe it and click **Commit version & share**. If a teammate committed in the meantime, their changes are merged in first.
- When a teammate commits, DAWGit tells you. Click **Preview** to see what changed, **Get updates** to take it. Close the set in Live first, then reopen it.
- A yellow banner means you and a teammate are editing the same track right now: talk before you both commit.

### Set up a team

One person does this once. Pick one:

- **Team server** — on a computer that is on while you work (or a NAS): Start menu → **DAWGit → DAWGit Team Server**. It shows the address and token to share. Teammates outside your network need a VPN (e.g. Tailscale) or port forwarding.
- **Team storage** — create an S3-compatible bucket (e.g. Cloudflare R2) and an access key per member, then make a connection code for each with `dawgit connection-code`. No computer has to stay on.

Step-by-step instructions: [docs/team-setup.md](docs/team-setup.md).

## Known limitations

- Windows only. macOS is planned.
- Tested with Ableton Live 12 (12.3). Other versions may work but are not verified.
- Some plugins store changing state even when you did not touch them, which can show up as a change or a conflict on that track.
- Deleted projects and old data are not cleaned up from team storage yet, so it only grows.
- No automatic updates: download new versions from the Releases page.

## More

- [Team setup guide](docs/team-setup.md)
- [Command line tool](docs/cli.md)
- [Building and development](docs/development.md)

Feedback and bug reports are welcome in [Issues](../../issues).

## License

[MIT](LICENSE)
