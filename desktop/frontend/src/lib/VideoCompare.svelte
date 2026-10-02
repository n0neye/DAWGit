<script lang="ts">

  // A video to watch; comparing, both side by side, played together with one
  // set of controls.
  type Take = { label: string; src: string };
  let { a, b, compare, onopen }: { a: Take | null; b: Take | null; compare: boolean; onopen?: () => void } = $props();
  let comparing = $derived(compare);

  let both = $derived(!!a && !!b);
  let main = $derived(a ?? b);
  let failed = $state<Record<string, boolean>>({});

  // Compare: two players kept together.
  let va = $state<HTMLVideoElement>();
  let vb = $state<HTMLVideoElement>();
  let playing = $state(false);
  let time = $state(0);
  let duration = $state(0);
  let muted = $state(true);
  function toggle() {
    if (!va || !vb) return;
    if (playing) { va.pause(); vb.pause(); playing = false; return; }
    vb.currentTime = va.currentTime;
    Promise.all([va.play(), vb.play()]).catch(() => {});
    playing = true;
  }
  function seek(t: number) {
    for (const v of [va, vb]) if (v) v.currentTime = Math.min(t, v.duration || t);
    time = t;
  }
  $effect(() => {
    if (!comparing || !both) return;
    let raf = 0;
    const tick = () => {
      if (va && vb) {
        time = va.currentTime;
        // b follows a; a little drift is corrected.
        if (playing && Math.abs(vb.currentTime - va.currentTime) > 0.15 && va.currentTime <= (vb.duration || 0)) {
          vb.currentTime = va.currentTime;
        }
        if (va.ended) playing = false;
      }
      raf = requestAnimationFrame(tick);
    };
    raf = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(raf);
  });
  $effect(() => { if (!comparing) playing = false; });
  const clock = (s: number) => `${Math.floor(s / 60)}:${String(Math.floor(s % 60)).padStart(2, "0")}`;
</script>

{#snippet player(t: Take, cls: string, bind?: "a" | "b")}
  {#if failed[t.src]}
    <div class="none {cls}">This video's format can't be played here
      {#if onopen && cls !== "small"}<button class="link" onclick={onopen}>Open in your video player</button>{/if}</div>
  {:else if bind === "a"}
    <video class={cls} src={t.src} bind:this={va} {muted} preload="metadata"
      onloadedmetadata={() => (duration = va?.duration ?? 0)} onerror={() => (failed[t.src] = true)}></video>
  {:else if bind === "b"}
    <video class={cls} src={t.src} bind:this={vb} {muted} preload="metadata" onerror={() => (failed[t.src] = true)}></video>
  {:else if cls === "small"}
    <video class={cls} src={`${t.src}#t=0.5`} muted preload="metadata" onerror={() => (failed[t.src] = true)}></video>
  {:else}
    <!-- svelte-ignore a11y_media_has_caption (project files have no captions) -->
    <video class={cls} src={t.src} controls preload="metadata" onerror={() => (failed[t.src] = true)}></video>
  {/if}
{/snippet}

<div class="vc">
  {#if both && comparing}
    <div class="pair">
      <figure><figcaption>{a!.label}</figcaption><div class="frame">{@render player(a!, "", "a")}</div></figure>
      <figure><figcaption>{b!.label}</figcaption><div class="frame">{@render player(b!, "", "b")}</div></figure>
    </div>
    <div class="controls">
      <button class="play" onclick={toggle} aria-label={playing ? "Pause" : "Play"}>{playing ? "❚❚" : "▶"}</button>
      <input type="range" min="0" max={duration || 0} step="0.01" value={time}
        oninput={(e) => seek(+(e.currentTarget as HTMLInputElement).value)} aria-label="Position" />
      <span class="faint time">{clock(time)} / {clock(duration)}</span>
      <button class="ghost" onclick={() => (muted = !muted)}>{muted ? "Sound off" : "Sound on"}</button>
    </div>
  {:else if main}
    <figure><figcaption>{main.label}</figcaption><div class="frame">{@render player(main, "")}</div></figure>
  {/if}
</div>

<style>
  .vc { display: flex; flex-direction: column; gap: 10px; }
  .bar { display: flex; align-items: center; gap: 12px; }
  figure { margin: 0; display: flex; flex-direction: column; gap: 6px; min-width: 0; }
  figcaption { font-size: 12px; text-transform: uppercase; letter-spacing: .06em; color: var(--muted); }
  .frame { border: 1px solid var(--line); border-radius: 8px; overflow: hidden; background: #000; line-height: 0; }
  .frame video { display: block; width: 100%; max-height: 58vh; background: #000; }
  .pair { display: grid; grid-template-columns: 1fr 1fr; gap: 10px; }
  .controls { display: flex; align-items: center; gap: 10px; }
  .controls input { flex: 1; }
  .play { width: 34px; padding: 4px 0; }
  .time { font-size: 12px; font-variant-numeric: tabular-nums; }
  .none { padding: 30px; text-align: center; color: var(--faint); font-size: 13px; line-height: 1.6; background: var(--bg);
    display: flex; flex-direction: column; align-items: center; gap: 4px; }
  .none.small { width: 56px; height: 32px; padding: 2px; font-size: 8px; }
</style>
