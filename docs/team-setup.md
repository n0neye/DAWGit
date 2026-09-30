# Setting up DAWGit for your team (Windows)

DAWGit keeps the version history of your Ableton Live projects and lets 2–3 people work on the same song. The team's songs live in a storage bucket the team owns (Cloudflare R2); everyone installs the **DAWGit app**. Nobody has to keep a computer running.

## 1. Install

Run `DAWGit-<version>-setup.exe` on every computer. No administrator rights are needed. Leave **Start with Windows** checked: DAWGit then waits in the system tray, backs up your work in progress, shows what teammates are editing and tells you about new versions. It never changes your project files on its own.

Requires Windows 10 (21H2 or later) or Windows 11.

## 2. Create the team (one person)

This takes about 5 minutes. You need a Cloudflare account; a small team usually stays within R2's free allowance, but Cloudflare may ask for a payment method when you activate R2.

In DAWGit choose **Create a team** (the first time DAWGit opens, or later from the Team menu → **Join/Create a Team…**). It walks you through these steps:

1. **Open Cloudflare R2.** Log in to the [Cloudflare dashboard](https://dash.cloudflare.com/) and open **R2 Object Storage**. The first time, activate R2.
2. **Create a bucket.** **Create bucket** → a name such as `night-shift-dawgit` → Location: *Automatic* → **Create bucket**. Use a bucket just for DAWGit.
3. **Create a key for the bucket.** On the R2 page: **Manage API tokens** → **Create API token** → Permissions: *Object Read & Write* → *Apply to specific buckets only*, and pick your bucket → **Create**. Copy the **Access Key ID**, the **Secret Access Key** (shown only once) and the **endpoint** for S3 clients (`https://<account id>.r2.cloudflarestorage.com`). The names in the dashboard may differ slightly.
4. **Name your team** and click **Check & create team.** DAWGit reads, writes and removes a test file to make sure everything works, then shows the team's **connection code**.

Send the connection code to each teammate **privately** (a direct message, not a public channel): it contains the key, and anyone who has it can read and change the team's songs. If a code leaks, create a new API token in Cloudflare, delete the old one, and enter the new key in the team's settings (below); then send teammates the new code.

Other S3-compatible storage works too if it supports conditional writes (`If-None-Match`), such as Amazon S3: click **Using other S3-compatible storage?** to set the folder and region. DAWGit checks this when you create the team.

## 3. Join the team (everyone else)

The first time DAWGit opens, it walks you through three steps:

1. **Team** — choose **Join a team** and paste the connection code.
2. **Your name** — shown next to the versions you commit.
3. **Projects** — download the songs you work on, or add a project folder of your own.

The **Team** menu at the top of the sidebar switches between teams and joins or creates another one (**Join/Create a Team…**). The **⚙** next to a team opens its settings:

- **Name** — **Rename for everyone** changes it in the team's storage and every member's DAWGit follows; **Only on this computer** keeps your own name for it.
- **Invite teammates** — copy the connection code again.
- **Connection** — change the bucket or key (after making a new key in Cloudflare). DAWGit checks the new settings before saving them.
- **Disconnect** — forget the team on this computer. Project folders stay.

Keys are stored once per computer (in `%APPDATA%\DAWGit\teams.json`), never inside project folders, so a project folder can be copied or shared without leaking them.

## 4. Share a project (the person who has it)

In the sidebar, click **+ Add project** and choose the Ableton project folder (the one with the `.als` file and `Ableton Project Info`). DAWGit commits a first version and uploads it, samples included.

To keep a project's versions on this computer only, pick **Local** in the Team menu and add it there. It can be shared later with **Share with a team…** at the top of the project.

## 5. Get a project (everyone else)

The sidebar lists every song in the current team. Songs not on this computer yet are dimmed (☁): pick one and click **Download**. Samples that lived outside the project on the other computer are downloaded too, and the set is pointed at them.

If you move a downloaded project folder, DAWGit marks it with ⚠: click **Locate folder…** to point it at the new place.

The **⋯** menu next to a song removes it from this computer's list (the folder stays), or deletes it from the team for everyone (you type the song's name to confirm). Copies already on someone's computer are kept.

## Everyday use

- Work in Live as usual and press **Ctrl+S**. Your changes appear under **Changes**, track by track.
- When you reach a point worth sharing, describe it and click **Commit version & share**. If a teammate committed in the meantime, DAWGit shows what they changed and lets you choose: combine your work with theirs (track by track; where you both changed the same track, it asks which to keep: yours, theirs, or both as two tracks), put your work on a new branch, or discard it.
- When a teammate saves, DAWGit shows it. Click **Preview** to see what changed, **Get updates** to take it. Close the set in Live first: DAWGit changes the files on disk and Live would overwrite them.
- A yellow banner means you and a teammate are editing the same track right now: talk before you both save.

## Uninstall

Settings → Apps → DAWGit → Uninstall. Your projects and their `.dawgit` history folders are kept, and so is the team's storage. Tick **Remove my settings** to also forget your teams, access tokens and name on this computer (useful before handing the computer on, or to try a fresh install).
