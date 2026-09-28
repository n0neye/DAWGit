<script lang="ts">
  import Modal from "./Modal.svelte";
  import ChangeList from "./ChangeList.svelte";
  import { ago, type Preview } from "./api";

  let { title, preview, actionLabel, onconfirm, onclose, blocked = "" }: {
    title: string;
    preview: Preview;
    actionLabel: string;
    onconfirm: () => void;
    onclose: () => void;
    blocked?: string; // why the action cannot run now
  } = $props();

  let nothing = $derived(preview.action === "up-to-date" || preview.action === "ahead");
</script>

<Modal {title} {onclose} width={720}>
  {#if nothing}
    <p class="muted">Nothing new — you already have everything.</p>
  {:else}
    <h3>{preview.versions.length} new version{preview.versions.length === 1 ? "" : "s"}</h3>
    <ul class="versions">
      {#each preview.versions as v (v.id)}
        <li><span class="msg">{v.message || "(no description)"}</span>
          <span class="faint">{v.author} · {ago(v.time)}</span></li>
      {/each}
    </ul>
    <h3>What changes</h3>
    <ChangeList changes={preview.changes} />
    {#if preview.conflicts.length}
      <div class="conflicts">
        <strong>{preview.conflicts.length} thing{preview.conflicts.length === 1 ? "" : "s"} you also changed</strong>
        — you'll choose what to keep next:
        <ul>
          {#each preview.conflicts as c (c.key)}
            <li>{c.unit === c.file ? c.file : `${c.unit} (${c.file})`}</li>
          {/each}
        </ul>
      </div>
    {:else if preview.action === "merge" && !blocked}
      <p class="ok">No conflicts — your work and theirs combine automatically.</p>
    {/if}
    {#if blocked}<p class="blocked">{blocked}</p>{/if}
  {/if}
  {#snippet footer()}
    <button onclick={onclose}>{nothing ? "Close" : "Cancel"}</button>
    {#if !nothing}
      <button class="primary" disabled={!!blocked} onclick={onconfirm}>{actionLabel}</button>
    {/if}
  {/snippet}
</Modal>

<style>
  h3 { font-size: 13px; text-transform: uppercase; letter-spacing: .06em; color: var(--muted); margin: 14px 0 8px; }
  .versions { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 4px; }
  .versions li { display: flex; gap: 10px; }
  .msg { flex: 1; }
  .conflicts {
    margin-top: 14px; padding: 10px 12px; border-radius: 8px;
    background: var(--warn-bg); border: 1px solid #5a4623; color: #f0d9a8;
  }
  .conflicts ul { margin: 6px 0 0; padding-left: 20px; }
  .ok { color: var(--accent); margin-top: 14px; }
  .blocked { margin-top: 14px; padding: 10px 12px; border-radius: 8px; background: #1d2c38; border: 1px solid #2c4557; }
</style>
