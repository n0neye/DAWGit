<script lang="ts">
  import { t as tr } from "./i18n.svelte"; // t is a track or take here
  import { imageLook, setImageMode } from "./viewers/settings.svelte";

  // A design file: one large; comparing, both large, side by side or under a
  // slider (remembered).
  type Take = { label: string; src: string };
  let { a, b, compare }: { a: Take | null; b: Take | null; compare: boolean } = $props();
  let comparing = $derived(compare);
  let mode = $derived(imageLook.mode);

  let failed = $state<Record<string, boolean>>({});
  let split = $state(50); // slider position, %
  let both = $derived(!!a && !!b);
  let main = $derived(a ?? b);

  function drag(e: PointerEvent) {
    const box = (e.currentTarget as HTMLElement).getBoundingClientRect();
    const move = (ev: PointerEvent) => { split = Math.max(0, Math.min(100, ((ev.clientX - box.left) / box.width) * 100)); };
    move(e);
    const up = () => { window.removeEventListener("pointermove", move); window.removeEventListener("pointerup", up); };
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", up);
  }
</script>

{#snippet picture(t: Take, cls = "")}
  {#if failed[t.src]}
    <div class="none {cls}">{tr("No preview for this file")}</div>
  {:else}
    <img class={cls} src={t.src} alt={t.label} draggable="false" onerror={() => (failed[t.src] = true)} />
  {/if}
{/snippet}

<div class="ic">
  {#if both && comparing}
    <div class="bar">
      <div class="modes">
        <button class:on={mode === "side"} onclick={() => setImageMode("side")}>{tr("Side by side")}</button>
        <button class:on={mode === "slider"} onclick={() => setImageMode("slider")}>{tr("Slider")}</button>
      </div>
    </div>
  {/if}

  {#if both && comparing && mode === "side"}
    <div class="pair">
      {#each [a!, b!] as t}
        <figure><figcaption>{t.label}</figcaption><div class="frame">{@render picture(t)}</div></figure>
      {/each}
    </div>
  {:else if both && comparing}
    <div class="labels"><span>{b!.label}</span><span>{a!.label}</span></div>
    <div class="frame slider" role="slider" aria-valuenow={Math.round(split)} tabindex="0" onpointerdown={drag}
      onkeydown={(e) => { if (e.key === "ArrowLeft") split = Math.max(0, split - 5); if (e.key === "ArrowRight") split = Math.min(100, split + 5); }}>
      {@render picture(b!)}
      <div class="over" style:clip-path="inset(0 0 0 {split}%)">{@render picture(a!)}</div>
      <div class="handle" style:left="{split}%"></div>
    </div>
  {:else if main}
    <figure>
      <figcaption>{main.label}</figcaption>
      <div class="frame">{@render picture(main)}</div>
    </figure>
  {/if}
</div>

<style>
  .ic { display: flex; flex-direction: column; gap: 10px; }
  .bar { display: flex; align-items: center; gap: 12px; }
  .modes { display: flex; }
  .modes button { padding: 3px 9px; font-size: 12px; border-radius: 0; }
  .modes button:first-child { border-radius: 6px 0 0 6px; }
  .modes button:last-child { border-radius: 0 6px 6px 0; margin-left: -1px; }
  .modes button.on { background: var(--accent); color: var(--accent-ink); border-color: var(--accent); }
  figure { margin: 0; display: flex; flex-direction: column; gap: 6px; min-width: 0; }
  figcaption, .labels { font-size: 12px; text-transform: uppercase; letter-spacing: .06em; color: var(--muted); }
  .labels { display: flex; justify-content: space-between; }
  /* A checkerboard shows transparency. */
  .frame { position: relative; border: 1px solid var(--line); border-radius: 8px; overflow: hidden; line-height: 0;
    background: repeating-conic-gradient(#2a2c31 0% 25%, #222428 0% 50%) 50% / 16px 16px; }
  /* At its own size (small previews stay sharp), smaller when it doesn't fit. */
  .frame img { display: block; margin: 0 auto; max-width: 100%; max-height: 62vh; user-select: none; }
  .pair { display: grid; grid-template-columns: 1fr 1fr; gap: 10px; }
  .slider { cursor: ew-resize; touch-action: none; }
  .over { position: absolute; inset: 0; }

  .handle { position: absolute; top: 0; bottom: 0; width: 2px; margin-left: -1px; background: #fff;
    box-shadow: 0 0 6px rgba(0, 0, 0, .6); pointer-events: none; }
  .none { padding: 30px; text-align: center; color: var(--faint); font-size: 13px; line-height: 1.4; }
  .none.small { width: 48px; height: 32px; padding: 2px; font-size: 8px; }
</style>
