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

### Alternative: team storage instead of a server

Instead of running a server, a team can keep its data in an S3-compatible bucket (for example Cloudflare R2 or Backblaze B2). Nobody needs to keep a computer running. Whoever sets it up:

1. Creates a bucket and one access key per team member, limited to that bucket.
2. Creates a connection code for each member:

   ```
   dawgit connection-code --endpoint https://<account>.r2.cloudflarestorage.com --bucket <bucket> --access-key <key> --secret-key <secret>
   ```

   (`dawgit.exe` is in the `bin` folder of the DAWGit install folder.)
3. Sends each member their code privately — it contains their key.

Everywhere the app asks for a server address, paste the connection code instead (no token needed). Notices about teammates arrive a little later than with a server (every 20 seconds).

## 3. Connect (everyone)

The first time DAWGit opens, it walks you through three steps:

1. **Connect** — enter the team server address and token, or paste your connection code.
2. **Your name** — shown next to the versions you commit.
3. **Projects** — download the songs you work on, or add a project folder of your own.

The **Team** menu at the top of the sidebar connects to another team, switches between teams, and renames or disconnects them (**Manage teams…**). Everyone sees the name the team was given (`serve --name`, or `connection-code --name` for team storage), and it follows when that name changes; a name you give in **Manage teams…** applies to your computer only. Access tokens and keys are stored once per computer (in `%APPDATA%\DAWGit\teams.json`), never inside project folders, so a project folder can be copied or shared without leaking them.

## 4. Share a project (the person who has it)

In the sidebar, click **+ Add project** and choose the Ableton project folder (the one with the `.als` file and `Ableton Project Info`). DAWGit commits a first version and uploads it, samples included.

To keep a project's versions on this computer only, pick **Local** in the Team menu and add it there. It can be shared later with **Share with a team…** at the top of the project.

## 5. Get a project (everyone else)

The sidebar lists every song in the current team. Songs not on this computer yet are dimmed (☁): pick one and click **Download**. Samples that lived outside the project on the other computer are downloaded too, and the set is pointed at them.

If you move a downloaded project folder, DAWGit marks it with ⚠: click **Locate folder…** to point it at the new place.

The **⋯** menu next to a song removes it from this computer's list (the folder stays), or deletes it from the team server for everyone (you type the song's name to confirm). Copies already on someone's computer are kept; on a self-hosted server the deleted song is moved to the `trash` folder in the server's data folder.

## Everyday use

- Work in Live as usual and press **Ctrl+S**. Your changes appear under **Changes**, track by track.
- When you reach a point worth sharing, describe it and click **Commit version & share**. If a teammate saved in the meantime, their changes are merged in first; if you both changed the same track, DAWGit asks which to keep (yours, theirs, or both as two tracks).
- When a teammate saves, DAWGit shows it. Click **Preview** to see what changed, **Get updates** to take it. Close the set in Live first: DAWGit changes the files on disk and Live would overwrite them.
- A yellow banner means you and a teammate are editing the same track right now: talk before you both save.

## Uninstall

Settings → Apps → DAWGit → Uninstall. Your projects, their `.dawgit` history folders and the server data are kept. Tick **Remove my settings** to also forget your teams, access tokens and name on this computer (useful before handing the computer on, or to try a fresh install).
