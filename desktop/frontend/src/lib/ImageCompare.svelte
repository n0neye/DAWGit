<script lang="ts">
  // A design file now and before: the one you look at large, the other as a
  // small thumbnail. Compare (remembered) shows both large, side by side or
  // under a slider.
  type Take = { label: string; src: string };
  let { a, b }: { a: Take | null; b: Take | null } = $props();

  const KEY = "dawgit.compareImages", MODE = "dawgit.compareMode";
  const read = (k: string) => { try { return localStorage.getItem(k) ?? ""; } catch { return ""; } };
  const write = (k: string, v: string) => { try { localStorage.setItem(k, v); } catch { /* not remembered */ } };
  let comparing = $state(read(KEY) === "1");
  let mode = $state<"side" | "slider">(read(MODE) === "slider" ? "slider" : "side");
  function setComparing(on: boolean) { comparing = on; write(KEY, on ? "1" : "0"); }
  function setMode(m: "side" | "slider") { mode = m; write(MODE, m); }

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
    <div class="none {cls}">No preview for this file</div>
  {:else}
    <img class={cls} src={t.src} alt={t.label} draggable="false" onerror={() => (failed[t.src] = true)} />
  {/if}
{/snippet}

<div class="ic">
  {#if both}
    <div class="bar">
      <label class="toggle"><input type="checkbox" class="switch" checked={comparing}
        onchange={(e) => setComparing((e.currentTarget as HTMLInputElement).checked)} /> Compare</label>
      {#if comparing}
        <div class="modes">
          <button class:on={mode === "side"} onclick={() => setMode("side")}>Side by side</button>
          <button class:on={mode === "slider"} onclick={() => setMode("slider")}>Slider</button>
        </div>
      {:else}
        <button class="thumb" onclick={() => setComparing(true)} title="Compare with this">
          {@render picture(b!, "small")}
          <span>{b!.label}</span>
        </button>
      {/if}
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
  .toggle { display: flex; align-items: center; gap: 6px; margin: 0; font-size: 12.5px; color: var(--muted); cursor: pointer; }
  .switch { appearance: none; position: relative; width: 26px; height: 15px; margin: 0; flex: none; cursor: pointer;
    border: none; padding: 0; border-radius: 8px; background: #3a3d45; transition: background .15s; }
  .switch::after { content: ""; position: absolute; top: 2px; left: 2px; width: 11px; height: 11px; border-radius: 50%;
    background: #d8dae0; transition: transform .15s; }
  .switch:checked { background: var(--accent); }
  .switch:checked::after { transform: translateX(11px); background: #fff; }
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
  .thumb { margin-left: auto; display: flex; align-items: center; gap: 8px; padding: 3px 10px 3px 3px;
    border-radius: 8px; background: var(--panel); text-align: left; }
  .thumb :global(.small) { width: 48px; height: 32px; object-fit: contain; border-radius: 4px; display: block;
    background: repeating-conic-gradient(#2a2c31 0% 25%, #222428 0% 50%) 50% / 10px 10px; }
  .thumb span { font-size: 12px; color: var(--muted); }
  .none { padding: 30px; text-align: center; color: var(--faint); font-size: 13px; line-height: 1.4; }
  .none.small { width: 48px; height: 32px; padding: 2px; font-size: 8px; }
</style>
