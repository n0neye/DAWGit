# DAWGit

**English** | [繁體中文](README-cht.md)

Version control and collaboration for Ableton Live, made for musicians.

> **Work in progress.** DAWGit is an early preview for Windows, tested with Ableton Live 12. Expect rough edges and changes before 1.0, and keep your own backups of projects that matter.

![DAWGit showing the tracks changed in a project, ready to commit a version](docs/images/screenshot.png)

## Why DAWGit

**Made for musicians.** Press Ctrl+S in Live as usual; DAWGit shows what changed, track by track. Commit a version with a sentence, go back to any version, and try ideas on a branch. When bandmates work on the same song, their changes are merged track by track, and DAWGit only asks when two people changed the same track. No Git knowledge needed.

**Samples come along automatically.** Samples from anywhere on your disk are stored with each version and relinked on your teammates' computers. No more *Collect All and Save* or "media files missing".

**Open source, with storage you own.** DAWGit is free and MIT-licensed. Your team's songs live in your own S3-compatible bucket (Cloudflare R2, Amazon S3, MinIO, …), not on our servers. A small team usually stays within Cloudflare R2's free allowance, and nobody has to keep a computer running.

## Limitations

- **Ableton Live only**, and Windows only for now (macOS is planned).
- **Plugins are not synced.** DAWGit does not copy plugins or check their versions, and it cannot collect samples that a plugin loads from outside the project (e.g. inside Kontakt or Serum). If your teammates don't have the same plugins, freeze those tracks before you share, or use Live's own Simpler, Sampler and Drum Rack, which sync completely. Some plugins also save changing state even when untouched, which can show up as a change on that track.
- **Everyone with the connection code has full access.** The code contains the storage key: anyone who has it can read, change and delete all of the team's songs. Share it privately, only with people you trust. If it leaks, make a new key and send the new code.
- **Early preview.** Old versions and deleted songs are not cleaned out of storage yet, and there are no automatic updates (DAWGit tells you when a new version is out).

## Getting started

Download `DAWGit-<version>-setup.exe` from the [Releases](../../releases) page and run it (Windows 10 21H2 or later, or Windows 11; no administrator rights needed). The installer is not code-signed yet: if Windows shows "Windows protected your PC", click **More info → Run anyway**.

Every project lives in a team's storage, so its versions are safe if a drive fails. **On your own?** Create a team of one.

**Start a team (one person):** choose **Create a team** and follow the steps to create a Cloudflare R2 bucket and key (about 5 minutes), or enter any other S3-compatible storage. DAWGit checks it and gives you a **connection code** to send to your teammates.

**Join a team:** choose **Join a team** and paste the connection code you were sent. Then download the team's songs or add your own.

The [team setup guide](docs/team-setup.md) has the details and everyday use.

## More

- [Team setup guide](docs/team-setup.md)
- [Command line tool](docs/cli.md)
- [DAWGit for AI agents](docs/agents.md) (Claude Code, Codex, Cursor…)
- [Building and development](docs/development.md)

Feedback and bug reports are welcome in [Issues](../../issues).

## How it's built

DAWGit is developed with AI coding assistants (Claude), directed and reviewed by its maintainer: every change is read, tried and decided on by a person. Because it looks after people's work, it leans on checks rather than trust:

- The merge and diff of Live sets are pinned by golden files (made by an independent first implementation); stored formats (content hashes, chunk boundaries, version records) have golden tests and never change in ways older versions can't read.
- End-to-end tests run real teams against real cloud storage (save, update, conflicts, interrupted uploads, restores).
- Nothing rewrites a project file while Live has it open, and nothing deletes a teammate's work: updates keep your uncommitted changes, storage cleanup and restores only touch what no version uses or what is missing.

See [Building and development](docs/development.md) and the design notes in [docs/design](docs/design).

## License

[MIT](LICENSE)
