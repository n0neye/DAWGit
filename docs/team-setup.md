# Setting up DAWGit for your team (Windows)

DAWGit keeps the version history of your Ableton Live projects and lets 2–3 people work on the same song. The team's songs live in a storage bucket the team owns (Cloudflare R2 or any other S3-compatible storage); everyone installs the **DAWGit app**. Nobody has to keep a computer running.

## 1. Install

Run `DAWGit-<version>-setup.exe` on every computer. No administrator rights are needed. Leave **Start with Windows** checked: DAWGit then waits in the system tray and tells you about new versions. It never changes your project files on its own.

Requires Windows 10 (21H2 or later) or Windows 11.

## 2. Create the team (one person)

The team's songs live in a bucket on **any S3-compatible storage** that supports conditional writes: Cloudflare R2, Amazon S3, MinIO and others. DAWGit checks the storage before it creates the team. We suggest Cloudflare R2 to start: the app guides you through it, downloads are free, and a small team usually stays within its free allowance.

In DAWGit choose **Create a team** (the first time DAWGit opens, or later from the Team menu → **Join/Create a Team…**), then pick **Cloudflare R2** or **Other S3-compatible**.

### Cloudflare R2 (about 5 minutes)

1. **Open R2.** Log in to the [Cloudflare dashboard](https://dash.cloudflare.com/) (or sign up) and open **Storage & databases → R2 Object Storage** in the sidebar. The first time, Cloudflare asks you to activate R2; it may ask for a payment method even for the free allowance.
2. **Create a bucket.** **Create bucket** → a name such as `night-shift-dawgit` → keep Location *Automatic* and Storage Class *Standard* → **Create bucket**. Use a bucket just for DAWGit.
3. **Create a key for the bucket.** Back on the R2 page, under *Account Details*: **Manage API Tokens** → **Create Account API token**. Permissions: *Object Read & Write*. Specify bucket(s): *Apply to specific buckets only* → your bucket. Then create the token.
4. **Copy the S3 credentials.** The next page is shown only once. Under *Use the following credentials for S3 clients*, copy the **Access Key ID**, the **Secret Access Key** and the **Default** endpoint (`https://<account id>.r2.cloudflarestorage.com`, also shown as *S3 API* on the R2 page). The *Token value* at the top of that page is not needed.
5. **Name your team** and click **Check & create team.** DAWGit reads, writes and removes a test file to make sure everything works, then shows the team's **connection code**.

### Other S3-compatible storage

Create a bucket for DAWGit and a key that may read, write and delete objects in it. In DAWGit choose **Other S3-compatible** and enter the bucket, the keys, the endpoint (e.g. `https://s3.eu-central-1.amazonaws.com` for Amazon S3) and the region; the folder inside the bucket defaults to `dawgit`. The storage must support conditional writes (`If-None-Match`), which keeps two people from overwriting each other's versions; if it doesn't, DAWGit says so and does not create the team.

### Send the connection code

Send the connection code to each teammate **privately** (a direct message, not a public channel): it contains the key, and anyone who has it can read and change the team's songs. If a code leaks, create a new key (in Cloudflare: a new API token, then delete the old one), enter it in the team's settings (below), and send teammates the new code.

## 3. Join the team (everyone else)

The first time DAWGit opens, it walks you through three steps:

1. **Team** — choose **Join a team** and paste the connection code.
2. **Your name** — shown next to the versions you commit.
3. **Projects** — download the songs you work on, or add a project folder of your own.

The **Team** menu at the top of the sidebar switches between teams and joins or creates another one (**Join/Create a Team…**). The **⚙** next to a team opens its settings:

- **Name** — **Rename for everyone** changes it in the team's storage and every member's DAWGit follows; **Only on this computer** keeps your own name for it.
- **Your name in this team** — shown next to the versions you commit; changing it changes it on all your versions, for everyone.
- **Invite teammates** — copy the connection code again.
- **Connection** — change the bucket or key (e.g. after making a new key). DAWGit checks the new settings before saving them.
- **Disconnect** — forget the team on this computer. By default its projects keep their versions, set aside. When you join the team again, DAWGit lists the projects of that team it finds on this computer, to reconnect the ones you tick.

Keys are stored once per computer (in `%APPDATA%\DAWGit\teams.json`), never inside project folders, so a project folder can be copied or shared without leaking them.

**Disk space.** A team project's history lives in the team's storage. On your computer, the project's `.dawgit` folder keeps its Live Sets (small) but not copies of samples the team's storage already has: the project folder has the current ones, and older ones are downloaded when you go to an older version, listen to one, or restore a file. When you disconnect from a team, you can download the files of older versions too.

## 4. Share a project (the person who has it)

In the sidebar, click **+ Add project** and choose the project folder (for Live, the one with the `.als` file and `Ableton Project Info`). DAWGit asks whether to commit and share a first version now, or later, after you've looked through the files and ignored the folders or files you don't need (right-click › Ignore). Nothing is uploaded until you say so.

## 5. Get a project (everyone else)

The sidebar lists every song in the current team. Songs not on this computer yet are dimmed (☁): pick one and click **Download**. Samples that lived outside the project on the other computer are downloaded too, and the set is pointed at them.

If you move a downloaded project folder, DAWGit marks it with ⚠: click **Locate folder…** to point it at the new place.

The **⋯** menu next to a project: **Pin to top**, **Open in Live** (or the project's program), **Open folder** and **Settings…**. The project's settings (also the ⚙ by its title) hold:

- **Name** — rename it for everyone in the team; the folder keeps its name.
- **Rules** — which files are versioned (`.dawgit.yaml`).
- **Check project…** — read its whole history again, looking for damage.
- **Unlink folder** — DAWGit stops listing the folder here; nothing is deleted.
- **Delete from the team…** — delete it for everyone (you type its name to confirm). Copies already on someone's computer are kept.

## Back up the team

The team's storage holds everything, but one member should keep a second copy on a drive or NAS, in case the bucket or its keys are ever lost. Creating a team ends with this suggestion; later, it's in the team's settings (⚙) → **Backup** → **Choose a backup folder…** (an empty folder, or the team's earlier backup).

- DAWGit copies what's new once a day while it is open (the first time: everything, in the background). It never deletes anything from the backup.
- If the drive isn't connected, it tries again later; after a few days without a backup, the sidebar says so.
- Everyone sees who backs up the team in **Backup**. While nobody has backed it up in the last week, the sidebar suggests it (✕ puts it off for a week).
- From the command line (e.g. a scheduled task on a NAS): `dawgit backup run <folder>`.

## Everyday use

- Work in Live as usual and press **Ctrl+S**. Your changes appear under **Changes**, track by track.
- When you reach a point worth sharing, describe it and click **Commit version & share**. If a teammate committed in the meantime, DAWGit shows what they changed and lets you choose: combine your work with theirs (track by track; where you both changed the same track, it asks which to keep: yours, theirs, or both as two tracks), put your work on a new branch, or discard it.
- When a teammate saves, DAWGit shows it. Click **Preview** to see what changed, **Get updates** to take it. Close the set in Live first: DAWGit changes the files on disk and Live would overwrite them.
- To stay out of each other's way, work on your own branch (**⑂ → New branch from here…**) and merge it into the main branch when it's ready (History tab → **Merge** on its latest version), or use your own `.als` in the same project and combine the sets in Live later. Editing the same set on the same branch works too: DAWGit combines your changes track by track and only asks when you both changed the same track.

## Uninstall

Settings → Apps → DAWGit → Uninstall. Your projects and their `.dawgit` history folders are kept, and so is the team's storage. Tick **Remove my settings** to also forget your teams, access tokens and name on this computer (useful before handing the computer on, or to try a fresh install).
