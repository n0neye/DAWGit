<script lang="ts">
  import { t } from "./i18n.svelte";
  import type { State } from "./api";

  // The commit box under the Changes list: the description, and the button
  // that says what it commits (all the changes, or the ticked ones) and
  // where it goes (this computer, or the team too).
  let { st, message = $bindable(), busy, leftOut, oncommit }: {
    st: State;
    message: string;
    busy: string;
    leftOut: number; // changes unticked: they stay uncommitted
    oncommit: () => void;
  } = $props();

  // The commit shortcut as the keyboard says it.
  const isMac = /Mac|iPhone|iPad/.test(navigator.platform || navigator.userAgent);
  let unticked = $derived(leftOut ? " " + t("Unticked files stay uncommitted.") : "");
</script>

<textarea rows="3" bind:value={message} placeholder={t("What did you change? e.g. “New bassline in the chorus”")}
  onkeydown={(e) => { if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) oncommit(); }}></textarea>
{#if st.olderVersion}
  <p class="faint small older">{st.remoteUrl
    ? t("You're on an older version. Committing combines your changes with the latest version of “{branch}” (you'll see a preview first) — or start a new branch from here.", { branch: st.branch })
    : t("You're on an older version. Make it the latest version to commit changes, or go back to the latest version.")}</p>
{/if}
<button class="primary commit-btn" disabled={!message.trim() || !!busy || (!!st.olderVersion && !st.remoteUrl)
  || (st.changes.length > 0 && leftOut === st.changes.length)} onclick={oncommit}
  title={st.remoteUrl
    ? t("Commits the project folder and shares it with the team on “{branch}”. If others committed in the meantime, you'll see what they changed and choose how to combine first.", { branch: st.branch }) + unticked
    : t("Commits on this computer. Share the project with a team to work on it together.") + unticked}>
  <span>{busy === "save" || busy === "first-share" ? t("Committing…")
    : leftOut ? t(st.remoteUrl ? "Commit {done} of {total} & Share" : "Commit {done} of {total}", { done: st.changes.length - leftOut, total: st.changes.length })
    : st.remoteUrl ? t("Commit & Share") : t("Commit")}</span>
  <kbd>{isMac ? "⌘" : "Ctrl"} ↵</kbd>
</button>

<style>
  .small { font-size: 12px; margin: 10px 0 0; }
  .older { margin: 6px 0 0; }
  .commit-btn { width: 100%; margin-top: 8px; padding: 8px 14px; display: flex; align-items: center; justify-content: center; gap: 10px; }
  .commit-btn kbd { font: inherit; font-size: 11px; opacity: .75; padding: 1px 5px; border-radius: 4px; border: 1px solid currentColor; }
  /* nothing to commit yet (no message, no changes): outlined, still easy to see */
  .commit-btn:disabled { background: transparent; border: 1px solid var(--accent); color: var(--accent); opacity: .7; }
</style>
