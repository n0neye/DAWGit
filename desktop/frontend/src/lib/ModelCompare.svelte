<script lang="ts">
  import { onDestroy } from "svelte";
  import { createViewer, type ModelStats, type Viewer } from "./model3d";

  // A 3D model now and before: the one you look at in a viewer, the other a
  // small label. Compare (remembered, shared with pictures) shows both,
  // their cameras moving together.
  type Take = { label: string; src: string; resolve: (rel: string) => string };
  let { a, b, ext }: { a: Take | null; b: Take | null; ext: string } = $props();

  const KEY = "dawgit.compareImages";
  const read = () => { try { return localStorage.getItem(KEY) === "1"; } catch { return false; } };
  let comparing = $state(read());
  function setComparing(on: boolean) {
    comparing = on;
    try { localStorage.setItem(KEY, on ? "1" : "0"); } catch { /* not remembered */ }
  }
  let both = $derived(!!a && !!b);
  let main = $derived(a ?? b);

  // One viewer per canvas; each loads when its canvas and model are there.
  let canvasA = $state<HTMLCanvasElement>();
  let canvasB = $state<HTMLCanvasElement>();
  let stats = $state<Record<string, ModelStats | null>>({});
  let errors = $state<Record<string, string>>({});
  const viewers: Record<string, Viewer | undefined> = {};
  // What each slot shows (address, canvas), and a count of what it was
  // asked to show: a model that finishes loading after it was replaced goes.
  const shown: Record<string, string> = {};
  const mounted: Record<string, HTMLCanvasElement | undefined> = {};
  const generation: Record<string, number> = { a: 0, b: 0 };

  function mount(slot: "a" | "b", canvas: HTMLCanvasElement | undefined, take: Take | null) {
    // The same model on the same canvas (the list reloaded): leave it be,
    // loaded or loading.
    const id = canvas && take ? take.src : "";
    if (shown[slot] === id && mounted[slot] === canvas) return;
    shown[slot] = id;
    mounted[slot] = canvas;
    const gen = ++generation[slot];
    viewers[slot]?.dispose();
    viewers[slot] = undefined;
    stats[slot] = null;
    errors[slot] = "";
    if (!canvas || !take) return;
    createViewer(canvas, take.src, ext, take.resolve).then((v) => {
      if (generation[slot] !== gen) { v.dispose(); return; }
      viewers[slot] = v;
      stats[slot] = v.stats;
      const other = slot === "a" ? "b" : "a";
      v.onCamera((s) => viewers[other]?.setCamera(s));
    }).catch((e) => { if (generation[slot] === gen) errors[slot] = e?.message ?? String(e); });
  }
  $effect(() => mount("a", canvasA, comparing && both ? a : main));
  $effect(() => mount("b", canvasB, comparing && both ? b : null));
  onDestroy(() => { viewers.a?.dispose(); viewers.b?.dispose(); });

  const fmt = (n: number) => (n >= 100 ? n.toFixed(0) : n >= 10 ? n.toFixed(1) : n.toFixed(2));
  const describe = (s: ModelStats | null | undefined) =>
    s ? `${s.triangles.toLocaleString()} triangles · ${s.size.map(fmt).join(" × ")}` : "";
</script>

{#snippet view(slot: "a" | "b", t: Take)}
  <figure>
    <figcaption><span>{t.label}</span><span class="stats">{describe(stats[slot])}</span></figcaption>
    <div class="frame">
      {#if slot === "a"}<canvas bind:this={canvasA}></canvas>{:else}<canvas bind:this={canvasB}></canvas>{/if}
      {#if errors[slot]}<div class="none">Can't show this model: {errors[slot]}</div>
      {:else if !stats[slot]}<div class="none faint">Loading the model…</div>{/if}
    </div>
  </figure>
{/snippet}

<div class="mc">
  {#if both}
    <div class="bar">
      <label class="toggle"><input type="checkbox" class="switch" checked={comparing}
        onchange={(e) => setComparing((e.currentTarget as HTMLInputElement).checked)} /> Compare</label>
      {#if !comparing}
        <button class="chip" onclick={() => setComparing(true)} title="Compare with this">{b!.label}</button>
      {/if}
      <span class="faint hint">Drag to turn · right-drag to move · wheel to zoom</span>
    </div>
  {:else if main}
    <div class="bar"><span class="faint hint">Drag to turn · right-drag to move · wheel to zoom</span></div>
  {/if}
  {#if both && comparing}
    <div class="pair">{@render view("a", a!)}{@render view("b", b!)}</div>
  {:else if main}
    {@render view("a", main)}
  {/if}
</div>

<style>
  .mc { display: flex; flex-direction: column; gap: 10px; }
  .bar { display: flex; align-items: center; gap: 12px; }
  .hint { margin-left: auto; font-size: 11.5px; }
  .toggle { display: flex; align-items: center; gap: 6px; margin: 0; font-size: 12.5px; color: var(--muted); cursor: pointer; }
  .switch { appearance: none; position: relative; width: 26px; height: 15px; margin: 0; flex: none; cursor: pointer;
    border: none; padding: 0; border-radius: 8px; background: #3a3d45; transition: background .15s; }
  .switch::after { content: ""; position: absolute; top: 2px; left: 2px; width: 11px; height: 11px; border-radius: 50%;
    background: #d8dae0; transition: transform .15s; }
  .switch:checked { background: var(--accent); }
  .switch:checked::after { transform: translateX(11px); background: #fff; }
  .chip { padding: 3px 10px; font-size: 12px; color: var(--muted); background: var(--panel); border-radius: 8px; }
  figure { margin: 0; display: flex; flex-direction: column; gap: 6px; min-width: 0; }
  figcaption { display: flex; justify-content: space-between; gap: 8px; font-size: 12px; text-transform: uppercase;
    letter-spacing: .06em; color: var(--muted); }
  .stats { text-transform: none; letter-spacing: 0; color: var(--faint); font-variant-numeric: tabular-nums; }
  .frame { position: relative; height: 52vh; min-height: 260px; border: 1px solid var(--line); border-radius: 8px;
    overflow: hidden; background: radial-gradient(circle at 50% 40%, #2a2d34, #17181c); }
  canvas { display: block; width: 100%; height: 100%; touch-action: none; }
  .pair { display: grid; grid-template-columns: 1fr 1fr; gap: 10px; }
  .pair .frame { height: 44vh; }
  .none { position: absolute; inset: 0; display: flex; align-items: center; justify-content: center; padding: 20px;
    text-align: center; font-size: 13px; color: var(--faint); pointer-events: none; }
</style>
