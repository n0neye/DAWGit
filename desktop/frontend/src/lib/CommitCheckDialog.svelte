<script lang="ts">
  import { t, tn } from "./i18n.svelte";
  import Modal from "./Modal.svelte";

  // Before a commit: missing samples DAWGit has copies of (restore them
  // first?), what the project's checks warn about (a Unity asset without its
  // .meta), and files the rules now leave out.
  let { leaving, warnings, restorable, missingSamples, oncommit, onrestore, onclose }: {
    leaving: string[]; // no longer tracked
    warnings: string[];
    restorable: number;
    missingSamples: number;
    oncommit: () => void; // as it is
    onrestore: () => void; // the samples, then commit
    onclose: () => void;
  } = $props();
  const MAX = 12;
</script>

<Modal title={warnings.length || restorable ? t("Before you commit") : t("No longer tracked")} {onclose}>
  {#if restorable}
    <p class="restore-note">⚠ {tn(missingSamples, "{n} sample is missing.", "{n} samples are missing.")}
      {tn(restorable, "DAWGit has a copy: restore it into the project first, so the team hears the same.", "DAWGit has copies of {n}: restore them into the project first, so the team hears the same.")}</p>
  {/if}
  {#if warnings.length}
    <ul class="warnings">
      {#each warnings.slice(0, MAX) as w}<li>⚠ {w}</li>{/each}
      {#if warnings.length > MAX}<li class="faint">… {t("and {n} more", { n: warnings.length - MAX })}</li>{/if}
    </ul>
  {/if}
  {#if leaving.length}
    <p>{tn(leaving.length, "The project's rules now leave this file out of versions. It stays on this computer, and on your teammates' computers too.",
      "The project's rules now leave these {n} files out of versions. They stay on this computer, and on your teammates' computers too.")}</p>
    <ul class="untracked mono">
      {#each leaving.slice(0, MAX) as f}<li>{f}</li>{/each}
      {#if leaving.length > MAX}<li class="faint">… {t("and {n} more", { n: leaving.length - MAX })}</li>{/if}
    </ul>
  {/if}
  {#snippet footer()}
    <button onclick={onclose}>{t("Cancel")}</button>
    {#if restorable}
      <button onclick={oncommit}>{t("Commit without them")}</button>
      <button class="primary" onclick={onrestore}>{t("Restore and commit")}</button>
    {:else}
      <button class="primary" onclick={oncommit}>{warnings.length ? t("Commit anyway") : t("Commit")}</button>
    {/if}
  {/snippet}
</Modal>

<style>
  .untracked { list-style: none; padding: 8px 12px; margin: 10px 0 0; background: var(--bg); border-radius: 8px;
    font-size: 12.5px; max-height: 220px; overflow: auto; }
  .warnings { list-style: none; padding: 0; margin: 0 0 12px; display: flex; flex-direction: column; gap: 6px;
    color: var(--warn); font-size: 13.5px; user-select: text; }
  .restore-note { color: #f0d9a8; }
</style>
