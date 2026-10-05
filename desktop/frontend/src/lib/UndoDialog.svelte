<script lang="ts">
  import { untrack } from "svelte";
  import { t, tn } from "./i18n.svelte";
  import { api, errorText, type Version } from "./api";
  import Modal from "./Modal.svelte";

  // Undo commit: a new version that takes back what one version changed,
  // keeping what came after it. Shown first: the files it changes,
  // uncommitted changes in the way (to commit or discard first), and whether
  // later versions changed the same things (decided after, as conflicts).
  let { root, version, onundo, onclose }: {
    root: string;
    version: Version;
    onundo: (message: string) => void;
    onclose: () => void;
  } = $props();

  type Plan = Awaited<ReturnType<typeof api.PlanUndo>>;
  let plan = $state<Plan>(null);
  let error = $state("");
  let message = $state(untrack(() => t("Undo “{version}”", { version: version.message || version.short })));

  $effect(() => {
    api.PlanUndo(root, version.id).then((p) => (plan = p)).catch((e) => (error = errorText(e)));
  });

  const MAX = 12;
</script>

<Modal title={t("Undo “{version}”?", { version: version.message || version.short })} {onclose}>
  <p class="muted small">{t("Makes a new version that takes back what this version changed. Versions after it keep their changes, and the history keeps everything.")}</p>
  {#if error}
    <p class="error small">{error}</p>
  {:else if !plan}
    <p class="faint small">{t("Working out what changes…")}</p>
  {:else}
    {#if plan.changed.length}
      <p class="small">{tn(plan.changed.length, "{n} file goes back:", "{n} files go back:")}</p>
      <ul class="files">
        {#each plan.changed.slice(0, MAX) as f}<li class="mono">{f}</li>{/each}
        {#if plan.changed.length > MAX}<li class="faint">… {t("and {n} more", { n: plan.changed.length - MAX })}</li>{/if}
      </ul>
    {/if}
    {#if plan.conflicts.length}
      <p class="warn small">{tn(plan.conflicts.length, "A later version changed {n} of the same things: you'll choose what to keep.", "Later versions changed {n} of the same things: you'll choose what to keep.")}</p>
    {/if}
    {#if plan.blocked.length}
      <p class="warn small">{t("You have uncommitted changes in files this undo changes. Commit or discard them first:")}</p>
      <ul class="files">{#each plan.blocked as f}<li class="mono">{f}</li>{/each}</ul>
    {:else}
      <label class="msg">{t("Message")}<input bind:value={message} /></label>
      <p class="faint small">{t("Your other uncommitted changes stay as they are.")}</p>
    {/if}
  {/if}
  {#snippet footer()}
    <button onclick={onclose}>{t("Cancel")}</button>
    <button class="primary" disabled={!plan || !!plan.blocked.length || !message.trim()}
      onclick={() => onundo(message.trim())}>{t("Undo commit")}</button>
  {/snippet}
</Modal>

<style>
  .small { font-size: 12.5px; }
  .files { margin: 4px 0 10px; padding-left: 18px; font-size: 12.5px; max-height: 220px; overflow: auto; user-select: text; }
  .mono { font-family: var(--mono, monospace); }
  .warn { color: var(--warn); }
  .error { color: var(--danger); user-select: text; }
  .msg { display: flex; flex-direction: column; gap: 4px; font-size: 13px; margin-top: 10px; }
</style>
