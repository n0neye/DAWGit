<script lang="ts">
  import { t as tr } from "./i18n.svelte"; // t is a track or take here
  import { onDestroy } from "svelte";
  import { createViewer, type ModelStats, type Viewer } from "./model3d";

  // A 3D model now and before: the one you look at in a viewer, the other a
  // small label. Compare (remembered, shared with pictures) shows both,
  // their cameras moving together.
  type Take = { label: string; src: string; resolve: (rel: string) => string };
  let { a, b, ext, compare }: { a: Take | null; b: Take | null; ext: string; compare: boolean } = $props();

  let comparing = $derived(compare);
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
      {:else if !stats[slot]}<div class="none faint">{tr("Loading the model…")}</div>{/if}
    </div>
  </figure>
{/snippet}

<div class="mc">
  {#if main}
    <div class="bar"><span class="faint hint">{tr("Drag to turn · right-drag to move · wheel to zoom")}</span></div>
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
