<script lang="ts">
  import { progressDetail, progressFraction, progressText, type Progress } from "./api";

  // A long step: what is happening, files, bytes, speed and time left.
  // Without p (not started yet) it shows `waiting` and a moving bar.
  let { p, team, waiting = "Working…" }: { p: Progress | null; team?: string; waiting?: string } = $props();

  let detail = $derived(p ? progressDetail(p) : "");
  let known = $derived(!!p && (!!p.totalBytes || !!p.total));
</script>

<div class="progress">
  <div class="text">
    <span>{p ? progressText(p, team) : waiting}</span>
    {#if detail}<span class="faint detail">{detail}</span>{/if}
  </div>
  <div class="bar" class:indeterminate={!known}>
    <div style="width: {known ? Math.round(progressFraction(p!) * 100) : 30}%"></div>
  </div>
</div>

<style>
  .progress { flex: 1; display: flex; flex-direction: column; gap: 6px; font-size: 13px; min-width: 0; }
  .text { display: flex; flex-wrap: wrap; gap: 4px 12px; }
  .detail { font-variant-numeric: tabular-nums; }
  .bar { height: 4px; border-radius: 2px; background: var(--line); overflow: hidden; }
  .bar > div { height: 100%; background: var(--accent); transition: width .2s; }
  .bar.indeterminate > div { animation: slide 1.2s ease-in-out infinite; }
  @keyframes slide { from { transform: translateX(-100%); } to { transform: translateX(340%); } }
</style>
