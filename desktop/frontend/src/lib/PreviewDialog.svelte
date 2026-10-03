<script lang="ts">
  import { t, tn } from "./i18n.svelte";
  import Modal from "./Modal.svelte";
  import IncomingChanges from "./IncomingChanges.svelte";
  import { type Preview } from "./api";

  let { root, title, preview, actionLabel, onconfirm, onclose, blocked = "", message = $bindable(null) }: {
    root: string;
    title: string;
    preview: Preview;
    actionLabel: string;
    onconfirm: () => void;
    onclose: () => void;
    blocked?: string; // why the action cannot run now
    // The new version's description (merges), editable; null: none.
    message?: string | null;
  } = $props();

  // Only a real merge makes a new version (a fast-forward takes theirs).
  let asksMessage = $derived(message !== null && preview.action === "merge");

  let nothing = $derived(preview.action === "up-to-date" || preview.action === "ahead");
  // Merges only combine the other versions: not listed (unless that's all).
  let versions = $derived.by(() => {
    const own = preview.versions.filter((v) => v.parents.length < 2);
    return own.length ? own : preview.versions;
  });

  // What happens to the files here, counted.
  const isSet = (p: string) => /\.als$/i.test(p);
  let sets = $derived(preview.changes.filter((c) => isSet(c.path) && c.status !== "deleted").length);
  let count = (status: string) => preview.changes.filter((c) => !isSet(c.path) && c.status === status).length;
</script>

<Modal {title} {onclose} width={1000}>
  {#if nothing}
    <p class="muted">{t("Nothing new — you already have everything.")}</p>
  {:else}
    <IncomingChanges {root} {preview} {versions} />

    <div class="effects">
      <strong>{t("What happens to your files")}</strong>
      <ul>
        {#if sets}<li>{tn(sets, "{n} set is updated: if it's open in Live, open it again afterwards", "{n} sets are updated: if they're open in Live, open them again afterwards")}</li>{/if}
        {#if count("added")}<li>{tn(count("added"), "{n} file is added", "{n} files are added")}</li>{/if}
        {#if count("modified")}<li>{tn(count("modified"), "{n} file is replaced by theirs", "{n} files are replaced by theirs")}</li>{/if}
        {#if count("renamed")}<li>{tn(count("renamed"), "{n} file is moved", "{n} files are moved")}</li>{/if}
        {#if count("deleted")}<li>{tn(count("deleted"), "{n} file is deleted", "{n} files are deleted")}</li>{/if}
        <li class="faint">{t("Your current version stays in the history: you can go back to it any time.")}</li>
      </ul>
    </div>

    {#if preview.conflicts.length}
      <div class="conflicts">
        <strong>{tn(preview.conflicts.length, "{n} thing you also changed", "{n} things you also changed")}</strong>
        {t("— you'll choose what to keep next:")}
        <ul>
          {#each preview.conflicts as c (c.key)}
            <li>{c.unit === c.file ? c.file : `${c.unit} (${c.file})`}</li>
          {/each}
        </ul>
      </div>
    {:else if preview.action === "merge" && !blocked}
      <p class="ok">{t("No conflicts — your work and theirs combine automatically.")}</p>
    {/if}
    {#if asksMessage}
      <label for="merge-msg">{t("Description of the merge version")}</label>
      <textarea id="merge-msg" rows="2" bind:value={message}></textarea>
    {/if}
    {#if blocked}<p class="blocked">{blocked}</p>{/if}
  {/if}
  {#snippet footer()}
    <button onclick={onclose}>{nothing ? t("Close") : t("Cancel")}</button>
    {#if !nothing}
      <button class="primary" disabled={!!blocked || (asksMessage && !message?.trim())} onclick={onconfirm}>{actionLabel}</button>
    {/if}
  {/snippet}
</Modal>

<style>
  .effects { margin-top: 14px; padding: 10px 12px; border-radius: 8px; background: var(--panel); border: 1px solid var(--line); }
  .effects ul { margin: 6px 0 0; padding-left: 20px; }
  .conflicts {
    margin-top: 14px; padding: 10px 12px; border-radius: 8px;
    background: var(--warn-bg); border: 1px solid #5a4623; color: #f0d9a8;
  }
  .conflicts ul { margin: 6px 0 0; padding-left: 20px; }
  .ok { color: var(--accent); margin-top: 14px; }
  label[for="merge-msg"] { margin-top: 16px; }
  textarea { width: 100%; resize: vertical; }
  .blocked { margin-top: 14px; padding: 10px 12px; border-radius: 8px; background: #1d2c38; border: 1px solid #2c4557; }
</style>
