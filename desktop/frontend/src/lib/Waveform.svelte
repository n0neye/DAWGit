<script lang="ts">
  import { t } from "./i18n.svelte";
  // A sample's waveform: the played part lit, a playhead, the time under the
  // pointer; click to jump there. It fills its box (the player sizes it).
  let { src, audio, onduration }: {
    src: string; // the /dawgit-file URL of the sample
    audio: HTMLAudioElement | undefined;
    onduration?: (seconds: number) => void;
  } = $props();

  const N = 800;
  let canvas = $state<HTMLCanvasElement>();
  let box = $state<HTMLDivElement>();
  let wave = $state<{ duration: number; min: number[]; max: number[] } | null>(null);
  let failed = $state(false);
  let time = $state(0);
  let hover = $state<number | null>(null); // 0..1 across the waveform
  let boxWidth = $state(0);

  // WAV and AIFF come prepared from the app; other formats are decoded here.
  async function load(url: string) {
    wave = null;
    failed = false;
    try {
      const res = await fetch(url.replace("/dawgit-file?", "/dawgit-peaks?") + `&n=${N}`);
      if (res.ok) wave = await res.json();
      else if (res.status === 415) wave = await decode(url);
      else throw new Error(await res.text());
      onduration?.(wave!.duration);
    } catch {
      failed = true;
    }
  }

  async function decode(url: string) {
    const data = await (await fetch(url)).arrayBuffer();
    const buf = await new OfflineAudioContext(1, 1, 44100).decodeAudioData(data);
    const min = new Array(N).fill(0), max = new Array(N).fill(0);
    for (let c = 0; c < buf.numberOfChannels; c++) {
      const d = buf.getChannelData(c);
      for (let i = 0; i < d.length; i++) {
        const s = Math.floor((i * N) / d.length);
        if (d[i] < min[s]) min[s] = d[i];
        if (d[i] > max[s]) max[s] = d[i];
      }
    }
    return { duration: buf.duration, min, max };
  }

  $effect(() => {
    load(src);
  });

  // Follow playback smoothly.
  $effect(() => {
    const el = audio;
    if (!el) return;
    let frame = 0;
    const tick = () => {
      time = el.currentTime;
      if (!el.paused) frame = requestAnimationFrame(tick);
    };
    const start = () => { cancelAnimationFrame(frame); tick(); };
    for (const ev of ["play", "seeked", "timeupdate", "pause"]) el.addEventListener(ev, start);
    return () => {
      cancelAnimationFrame(frame);
      for (const ev of ["play", "seeked", "timeupdate", "pause"]) el.removeEventListener(ev, start);
    };
  });

  $effect(() => {
    const el = box;
    if (!el) return;
    const ro = new ResizeObserver(() => (boxWidth = el.clientWidth));
    ro.observe(el);
    return () => ro.disconnect();
  });

  $effect(() => {
    const c = canvas, w = wave;
    const at = time, cw = boxWidth; // redraw as it plays or resizes
    if (!c || !w || !cw) return;
    const dpr = window.devicePixelRatio || 1;
    const ch = c.clientHeight;
    c.width = Math.round(cw * dpr);
    c.height = Math.round(ch * dpr);
    const g = c.getContext("2d")!;
    g.scale(dpr, dpr);
    g.clearRect(0, 0, cw, ch);
    const css = getComputedStyle(c);
    const lit = css.getPropertyValue("--accent").trim() || "#3ecf9f";
    const dim = css.getPropertyValue("--wave").trim() || "#5b5f68";
    const mid = ch / 2, amp = mid - 2;
    const upTo = w.duration ? (at / w.duration) * cw : 0;
    const bar = 2, gap = 1; // bars, like most DAW overviews
    for (let x = 0; x < cw; x += bar + gap) {
      const from = Math.floor((x / cw) * w.min.length);
      const to = Math.max(from + 1, Math.floor(((x + bar) / cw) * w.min.length));
      let lo = 0, hi = 0;
      for (let s = from; s < to && s < w.min.length; s++) {
        lo = Math.min(lo, w.min[s]);
        hi = Math.max(hi, w.max[s]);
      }
      const top = mid - hi * amp, h = Math.max(1, (hi - lo) * amp);
      g.fillStyle = x + bar <= upTo ? lit : dim;
      g.fillRect(x, top, bar, h);
    }
    g.fillStyle = "rgba(255,255,255,.08)";
    g.fillRect(0, mid, cw, 1);
  });

  function frac(e: MouseEvent) {
    const r = canvas!.getBoundingClientRect();
    return Math.min(1, Math.max(0, (e.clientX - r.left) / r.width));
  }

  function seek(e: MouseEvent) {
    if (!audio || !wave) return;
    audio.currentTime = frac(e) * wave.duration;
    time = audio.currentTime;
  }

  function fmt(s: number): string {
    const m = Math.floor(s / 60), sec = s - m * 60;
    return `${m}:${sec.toFixed(1).padStart(4, "0")}`;
  }
</script>

<div class="wave" bind:this={box}>
  {#if failed}
    <div class="note">{t("No waveform for this file")}</div>
  {:else if !wave}
    <div class="note">{t("Reading the sample…")}</div>
  {:else}
    <canvas bind:this={canvas} onclick={seek} onmousemove={(e) => (hover = frac(e))}
      onmouseleave={() => (hover = null)}></canvas>
    {#if wave.duration}
      <div class="head" style:left="{(time / wave.duration) * 100}%"></div>
    {/if}
    {#if hover !== null}
      <div class="hover" style:left="{hover * 100}%"><span>{fmt(hover * wave.duration)}</span></div>
    {/if}
  {/if}
</div>

<style>
  .wave { position: relative; height: 100%; min-width: 0; --wave: #5b5f68; }
  canvas { display: block; width: 100%; height: 100%; cursor: pointer; }
  .head { position: absolute; top: 0; bottom: 0; width: 2px; margin-left: -1px; background: #fff; pointer-events: none;
    box-shadow: 0 0 6px rgba(255, 255, 255, .5); }
  .hover { position: absolute; top: 0; bottom: 0; width: 1px; background: rgba(255, 255, 255, .45); pointer-events: none; }
  .hover span { position: absolute; top: 2px; left: 5px; font-size: 10.5px; color: var(--text); background: rgba(0, 0, 0, .6);
    padding: 0 4px; border-radius: 3px; font-variant-numeric: tabular-nums; white-space: nowrap; }
  .note { font-size: 12px; color: var(--faint); height: 100%; display: flex; align-items: center; padding: 0 10px; }
</style>
