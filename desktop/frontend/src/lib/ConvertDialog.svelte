<script lang="ts">
  import { Events } from "@wailsio/runtime";
  import Modal from "./Modal.svelte";
  import ProgressBar from "./ProgressBar.svelte";
  import { api, errorText, type Progress } from "./api";

  // Convert a sample to another format; the new file goes next to it.
  let { root, file, onclose, ondone }: {
    root: string;
    file: string; // relative path of the sample
    onclose: () => void;
    ondone: (newFile: string) => void;
  } = $props();

  type Fmt = { id: string; name: string; ext: string; bitrates: number[] };
  let formats = $state<Fmt[]>([]);
  let format = $state("mp3");
  let kbps = $state(320);
  let target = $state("");
  let busy = $state(false);
  let error = $state("");
  let progress = $state<Progress | null>(null);

  let current = $derived(formats.find((f) => f.id === format));
  const name = (p: string) => p.slice(p.lastIndexOf("/") + 1);

  $effect(() => {
    api.ConvertFormats().then((f) => (formats = (f ?? []) as Fmt[]));
  });

  // The default bitrate for the chosen format, and where the file will go.
  $effect(() => {
    const f = current;
    if (!f) return;
    if (f.bitrates.length && !f.bitrates.includes(kbps)) kbps = f.bitrates[0];
    api.ConvertTarget(root, file, f.id).then((t) => (target = t)).catch(() => (target = ""));
  });

  $effect(() => Events.On("progress", (ev: { data: Progress }) => {
    if (busy && ev.data.root === root) progress = ev.data.stage === "done" ? null : ev.data;
  }));

  async function run() {
    busy = true;
    error = "";
    progress = null;
    try {
      ondone(await api.ConvertFile(root, file, format, current?.bitrates.length ? kbps : 0));
    } catch (e) {
      error = errorText(e);
    } finally {
      busy = false;
    }
  }
</script>

<Modal title="Convert {name(file)}" onclose={() => { if (!busy) onclose(); }}>
  <div class="grid">
    <label for="cf">Format</label>
    <select id="cf" bind:value={format} disabled={busy}>
      {#each formats as f (f.id)}<option value={f.id}>{f.name}</option>{/each}
    </select>

    {#if current?.bitrates.length}
      <label for="cb">Quality</label>
      <select id="cb" bind:value={kbps} disabled={busy}>
        {#each current.bitrates as b}<option value={b}>{b} kbps{b === current.bitrates[0] ? " (best)" : ""}</option>{/each}
      </select>
    {/if}
  </div>

  {#if target}<p class="muted small">Saved next to the original as <span class="mono">{name(target)}</span>. The original stays.</p>{/if}
  {#if busy}<ProgressBar p={progress} waiting="Starting…" />{/if}
  {#if error}<p class="error">{error}</p>{/if}

  {#snippet footer()}
    <button onclick={onclose} disabled={busy}>Cancel</button>
    <button class="primary" onclick={run} disabled={busy || !current}>{busy ? "Converting…" : "Convert"}</button>
  {/snippet}
</Modal>

<style>
  .grid { display: grid; grid-template-columns: auto 1fr; gap: 10px 14px; align-items: center; margin-bottom: 12px; }
  .grid label { margin: 0; }
  select { width: 100%; }
  .small { font-size: 12.5px; }
  .error { color: var(--danger); }
</style>
