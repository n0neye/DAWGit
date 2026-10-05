<script lang="ts">
  import { untrack } from "svelte";
  import { t, tn } from "./i18n.svelte";
  import { api, errorText, type Version } from "./api";
  import Modal from "./Modal.svelte";

  // Undo commit. Your latest version, while no one else has it, is taken
  // back: gone from the history, its changes uncommitted again. Otherwise a
  // new version takes back what it changed, keeping what came after it.
  // Shown first: the files it changes, uncommitted changes in the way (to
  // commit or discard first), and whether later versions changed the same
  // things (decided after, as conflicts).
  let { root, version, onundo, onclose }: {
    root: string;
    version: Version;
    onundo: (message: string, takeBack: boolean) => void;
    onclose: () => void;
  } = $props();

  type Plan = Awaited<ReturnType<typeof api.PlanUndo>>;
  let plan = $state<Plan>(null);
  let error = $state("");
  let label = untrack(() => version.message || version.short);
  let message = $state(t("Undo “{version}”", { version: label }));
  // Keep it in the history (a new version) even though it can be taken back.
  let keep = $state(false);

  $effect(() => {
    api.PlanUndo(root, version.id).then((p) => (plan = p)).catch((e) => (error = errorText(e)));
  });

  let takeBack = $derived(!!plan?.takeBack.ok && !keep);
  // Why the latest version stays in the history.
  let why = $derived.by(() => {
    const tb = plan?.takeBack;
    if (!tb) return "";
    if (tb.why === "has-it") return t("{who} already has this version, so it stays in the history: a new version takes it back.", { who: tb.haveIt.join(", ") });
    if (tb.why === "on-branch") return t("The branch {branch} has this version too, so it stays in the history: a new version takes it back.", { branch: tb.branches.join(", ") });
    if (tb.why === "who") return t("DAWGit can't tell who made this version (choose your name in the team first), so it stays in the history: a new version takes it back.");
    if (tb.why === "not-yours") return t("It's a teammate's version, so it stays in the history: a new version takes it back.");
    return "";
  });

  const MAX = 12;
</script>

<Modal title={t("Undo “{version}”?", { version: label })} {onclose}>
  {#if error}
    <p class="error small">{error}</p>
  {:else if !plan}
    <p class="faint small">{t("Working out what changes…")}</p>
  {:else if takeBack}
    <p class="small">{plan.takeBack.shared
      ? t("Removes it from the history, yours and the team's. Its changes come back as uncommitted changes, so you can fix them and commit again.")
      : t("Removes it from the history. Its changes come back as uncommitted changes, so you can fix them and commit again.")}</p>
    {#if plan.takeBack.featureOff}
      <p class="faint small">{t("This turns on undoing versions for the team: teammates with an older DAWGit are asked to update it.")}</p>
    {/if}
    {#if !plan.error}
      <button class="link small" onclick={() => (keep = true)}>{t("Keep it in the history instead")}</button>
    {/if}
  {:else if plan.error}
    <p class="error small">{plan.error}</p>
  {:else}
    {#if why}<p class="small">{why}</p>{/if}
    <p class="muted small">{t("Makes a new version that takes back what this version changed. Versions after it keep their changes, and the history keeps everything.")}</p>
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
    <button class="primary"
      disabled={!plan || (!takeBack && (!!plan.error || !!plan.blocked.length || !message.trim()))}
      onclick={() => onundo(message.trim(), takeBack)}>{t("Undo commit")}</button>
  {/snippet}
</Modal>

<style>
  .small { font-size: 12.5px; }
  .files { margin: 4px 0 10px; padding-left: 18px; font-size: 12.5px; max-height: 220px; overflow: auto; user-select: text; }
  .mono { font-family: var(--mono, monospace); }
  .warn { color: var(--warn); }
  .error { color: var(--danger); user-select: text; }
  .msg { display: flex; flex-direction: column; gap: 4px; font-size: 13px; margin-top: 10px; }
  .link { background: none; border: none; padding: 0; color: var(--accent); cursor: pointer; text-decoration: underline; }
</style>
