## Version control: DAWGit

This project is versioned with DAWGit (`.dawgit/`), not git.

- `dawgit status --json`: what changed; `dawgit update --preview --json`: what teammates saved.
- `dawgit save -m "message" --json` saves a version and shares it with the team.
- Commands never prompt: on `merge_conflict` (exit 3) show `error.conflicts` to the user and rerun with
  `--strategy ours|theirs|both` as they decide; on `set_open_in_live` (exit 4) ask them to close the set in Ableton Live.
- Never use `--force` without asking, never print connection codes (they hold storage keys), don't edit `.dawgit/`.
- Full guide: `dawgit help agents`.
