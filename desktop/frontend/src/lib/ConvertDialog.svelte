<script lang="ts">
  import { t } from "./i18n.svelte";
  import { Events } from "@wailsio/runtime";
  import Modal from "./Modal.svelte";
  import ProgressBar from "./ProgressBar.svelte";
  import { api, errorText, type Progress } from "./api";

  // Convert a sample to another format; the new file goes next to it.
  // Sample rate, channels and bit depth keep the original's unless changed;
  // notes say what a format forces.
  let { root, file, onclose, ondone }: {
    root: string;
    file: string; // relative path of the sample
    onclose: () => void;
    ondone: (newFile: string) => void;
  } = $props();

  type Fmt = { id: string; name: string; ext: string; bitrates: number[]; rates: number[]; bits: number[] };
  let formats = $state<Fmt[]>([]);
  let format = $state("mp3");
  let kbps = $state(320);
  let rate = $state(0); // 0: same as original
  let channels = $state(0);
  let bits = $state(0);
  let info = $state<{ rate: number; channels: number; bits: number; seconds: number } | null>(null);
  let plan = $state<{ rate: number; channels: number; bits: number; bitrate: number; notes: string[] } | null>(null);
  let target = $state("");
  let busy = $state(false);
  let error = $state("");
  let progress = $state<Progress | null>(null);

  let current = $derived(formats.find((f) => f.id === format));
  const name = (p: string) => p.slice(p.lastIndexOf("/") + 1);
  const khz = (hz: number) => `${hz % 1000 ? (hz / 1000).toFixed(1) : hz / 1000} kHz`;
  const chName = (n: number) => (n === 1 ? t("mono") : n === 2 ? t("stereo") : t("{n} channels", { n }));
  // Lossy output shows its bitrate in place of the bit depth.
  const describe = (x: { rate: number; channels: number; bits: number; bitrate?: number }) =>
    [x.bitrate && `${x.bitrate} kbps`, x.rate && khz(x.rate), !x.bitrate && x.bits && `${x.bits}-bit`, x.channels && chName(x.channels)]
      .filter(Boolean).join(" · ");

  $effect(() => {
    api.ConvertFormats().then((f) => (formats = (f ?? []) as Fmt[]));
    api.ConvertInfo(root, file).then((i) => (info = i)).catch(() => (info = null));
  });

  // Keep choices valid for the format; work out the output and the file name.
  $effect(() => {
    const f = current;
    if (!f) return;
    if (f.bitrates.length && !f.bitrates.includes(kbps)) kbps = f.bitrates[0];
    if (rate && !f.rates.includes(rate)) rate = 0;
    if (bits && !f.bits.includes(bits)) bits = 0;
    const [k, r, c, b] = [f.bitrates.length ? kbps : 0, rate, channels, bits];
    api.ConvertTarget(root, file, f.id).then((t) => (target = t)).catch(() => (target = ""));
    api.ConvertPlan(root, file, f.id, k, r, c, b).then((p) => { plan = p; error = ""; })
      .catch((e) => { plan = null; error = errorText(e); });
  });

  $effect(() => Events.On("progress", (ev: { data: Progress }) => {
    if (busy && ev.data.root === root) progress = ev.data.stage === "done" ? null : ev.data;
  }));

  async function run() {
    busy = true;
    error = "";
    progress = null;
    try {
      ondone(await api.ConvertFile(root, file, format, current?.bitrates.length ? kbps : 0, rate, channels, bits));
    } catch (e) {
      error = errorText(e);
    } finally {
      busy = false;
    }
  }
</script>

<Modal title={t("Convert {file}", { file: name(file) })} onclose={() => { if (!busy) onclose(); }}>
  {#if info}<p class="faint small orig">{t("Original:")} {describe(info)}{info.seconds ? ` · ${info.seconds.toFixed(1)} s` : ""}</p>{/if}

  <div class="grid">
    <label for="cf">{t("Format")}</label>
    <select id="cf" bind:value={format} disabled={busy}>
      {#each formats as f (f.id)}<option value={f.id}>{f.name}</option>{/each}
    </select>

    {#if current?.bitrates.length}
      <label for="cb">{t("Quality")}</label>
      <select id="cb" bind:value={kbps} disabled={busy}>
        {#each current.bitrates as b}<option value={b}>{b} kbps{b === current.bitrates[0] ? ` (${t("best")})` : ""}</option>{/each}
      </select>
    {/if}

    {#if current}
      <label for="cr">{t("Sample rate")}</label>
      <select id="cr" bind:value={rate} disabled={busy}>
        <option value={0}>{t("Same as original")}{info?.rate ? ` (${khz(info.rate)})` : ""}</option>
        {#each current.rates as r}<option value={r}>{khz(r)}</option>{/each}
      </select>

      <label for="cc">{t("Channels")}</label>
      <select id="cc" bind:value={channels} disabled={busy}>
        <option value={0}>{t("Same as original")}{info?.channels ? ` (${chName(info.channels)})` : ""}</option>
        <option value={2}>{t("Stereo")}</option>
        <option value={1}>{t("Mono (channels mixed)")}</option>
      </select>

      {#if current.bits.length}
        <label for="cd">{t("Bit depth")}</label>
        <select id="cd" bind:value={bits} disabled={busy}>
          <option value={0}>{t("Same as original")}{info?.bits ? ` (${info.bits}-bit)` : ""}</option>
          {#each current.bits as b}<option value={b}>{b}-bit</option>{/each}
        </select>
      {/if}
    {/if}
  </div>

  {#if plan}
    <p class="small result">{target ? t("Writes {what} as {file} next to the original. The original stays.", { what: describe(plan), file: name(target) })
      : t("Writes {what} next to the original. The original stays.", { what: describe(plan) })}</p>
    {#each plan.notes as n}<p class="small note">{n}</p>{/each}
  {/if}
  {#if busy}<ProgressBar p={progress} waiting={t("Starting…")} />{/if}
  {#if error}<p class="error small">{error}</p>{/if}

  {#snippet footer()}
    <button onclick={onclose} disabled={busy}>{t("Cancel")}</button>
    <button class="primary" onclick={run} disabled={busy || !current || !plan}>{busy ? "Converting…" : "Convert"}</button>
  {/snippet}
</Modal>

<style>
  .orig { margin: 0 0 12px; }
  .grid { display: grid; grid-template-columns: auto 1fr; gap: 10px 14px; align-items: center; margin-bottom: 10px; }
  .grid label { margin: 0; }
  select { width: 100%; }
  .small { font-size: 12.5px; }
  .result { color: var(--muted); margin: 10px 0 4px; }
  .note { color: var(--warn); margin: 2px 0; }
  .error { color: var(--danger); }
</style>
