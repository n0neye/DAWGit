<script lang="ts">
  // A sample's waveform with the played part lit and a playhead; click to jump
  // there. width is the share of the row it takes (two versions share one time
  // scale, so a shorter take draws shorter).
  let { src, audio, width = 1, onduration }: {
    src: string; // the /dawgit-file URL of the sample
    audio: HTMLAudioElement | undefined;
    width?: number;
    onduration?: (seconds: number) => void;
  } = $props();

  const N = 800;
  let canvas = $state<HTMLCanvasElement>();
  let wave = $state<{ duration: number; min: number[]; max: number[] } | null>(null);
  let failed = $state(false);
  let time = $state(0);

  // WAV and AIFF come prepared from the app; other formats are decoded here.
  async function load(url: string) {
    wave = null;
    failed = false;
    try {
      const res = await fetch(url.replace("/dawgit-file?", "/dawgit-peaks?") + `&n=${N}`);
      if (res.ok) {
        wave = await res.json();
      } else if (res.status === 415) {
        wave = await decode(url);
      } else throw new Error(await res.text());
      onduration?.(wave!.duration);
    } catch {
      failed = true;
    }
  }

  async function decode(url: string) {
    const data = await (await fetch(url)).arrayBuffer();
    const ctx = new OfflineAudioContext(1, 1, 44100);
    const buf = await ctx.decodeAudioData(data);
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

  // Follow playback.
  $effect(() => {
    const el = audio;
    if (!el) return;
    let frame = 0;
    const tick = () => {
      time = el.currentTime;
      if (!el.paused) frame = requestAnimationFrame(tick);
    };
    const start = () => { cancelAnimationFrame(frame); tick(); };
    el.addEventListener("play", start);
    el.addEventListener("seeked", start);
    el.addEventListener("timeupdate", start);
    return () => {
      cancelAnimationFrame(frame);
      el.removeEventListener("play", start);
      el.removeEventListener("seeked", start);
      el.removeEventListener("timeupdate", start);
    };
  });

  $effect(() => {
    const c = canvas, w = wave;
    const at = time; // redraw as it plays
    if (!c || !w) return;
    const dpr = window.devicePixelRatio || 1;
    const cw = c.clientWidth, ch = c.clientHeight;
    c.width = Math.round(cw * dpr);
    c.height = Math.round(ch * dpr);
    const g = c.getContext("2d")!;
    g.scale(dpr, dpr);
    g.clearRect(0, 0, cw, ch);
    const styles = getComputedStyle(c);
    const played = styles.getPropertyValue("--accent").trim() || "#3ecf9f";
    const rest = styles.getPropertyValue("--faint").trim() || "#666";
    const mid = ch / 2;
    const upTo = w.duration ? (at / w.duration) * cw : 0;
    for (let x = 0; x < cw; x++) {
      const s = Math.floor((x / cw) * w.min.length);
      const lo = w.min[s] ?? 0, hi = w.max[s] ?? 0;
      g.fillStyle = x < upTo ? played : rest;
      g.fillRect(x, mid - hi * mid, 1, Math.max(1, (hi - lo) * mid));
    }
    if (at > 0) {
      g.fillStyle = "#fff";
      g.fillRect(Math.min(upTo, cw - 1), 0, 1, ch);
    }
  });

  function seek(e: MouseEvent) {
    if (!audio || !wave || !canvas) return;
    const r = canvas.getBoundingClientRect();
    audio.currentTime = ((e.clientX - r.left) / r.width) * wave.duration;
    time = audio.currentTime;
  }
</script>

<div class="wave" style:width="{Math.max(0.05, Math.min(1, width)) * 100}%">
  {#if failed}
    <div class="note faint">No waveform for this file</div>
  {:else if !wave}
    <div class="note faint">Reading the sample…</div>
  {:else}
    <canvas bind:this={canvas} onclick={seek} title="Click to play from here"></canvas>
  {/if}
</div>

<style>
  .wave { height: 64px; background: var(--bg); border: 1px solid var(--line); border-radius: 6px; overflow: hidden; }
  canvas { display: block; width: 100%; height: 100%; cursor: pointer; }
  .note { font-size: 12px; padding: 22px 10px; }
</style>
