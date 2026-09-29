<script lang="ts">
  import Waveform from "./Waveform.svelte";

  // Two versions of a sample, one above the other, on one time scale. Starting
  // one pauses the other and picks up at the same position, to hear the
  // difference.
  let { a, b }: {
    a: { label: string; src: string } | null; // newer
    b: { label: string; src: string } | null; // older
  } = $props();

  let top = $state<HTMLAudioElement>();
  let bottom = $state<HTMLAudioElement>();
  let failed = $state<Record<string, boolean>>({});
  let durations = $state<Record<string, number>>({});
  let longest = $derived(Math.max(0, ...Object.values(durations)));

  function played(me: HTMLAudioElement | undefined, other: HTMLAudioElement | undefined) {
    if (!me || !other) return;
    if (!other.paused) {
      me.currentTime = Math.min(other.currentTime, me.duration || other.currentTime);
      other.pause();
    }
  }
</script>

<div class="ab">
  {#each [{ side: a, el: "top" }, { side: b, el: "bottom" }] as row (row.el)}
    {#if row.side}
      {@const side = row.side}
      <div class="take">
        <div class="label">
          {side.label}
          {#if durations[side.src]}<span class="faint dur">{durations[side.src].toFixed(1)} s</span>{/if}
        </div>
        {#if failed[side.src]}
          <p class="faint small">This file can't be played here.</p>
        {:else}
          <Waveform src={side.src} audio={row.el === "top" ? top : bottom}
            width={longest && durations[side.src] ? durations[side.src] / longest : 1}
            onduration={(d) => (durations[side.src] = d)} />
          {#if row.el === "top"}
            <audio bind:this={top} controls preload="metadata" src={side.src}
              onplay={() => played(top, bottom)} onerror={() => (failed[side.src] = true)}></audio>
          {:else}
            <audio bind:this={bottom} controls preload="metadata" src={side.src}
              onplay={() => played(bottom, top)} onerror={() => (failed[side.src] = true)}></audio>
          {/if}
        {/if}
      </div>
    {/if}
  {/each}
  {#if a && b}<p class="faint small">Start one while the other plays to switch at the same position. Click a waveform to jump.</p>{/if}
</div>

<style>
  .ab { display: flex; flex-direction: column; gap: 16px; }
  .take { display: flex; flex-direction: column; align-items: stretch; gap: 6px; }
  .label { font-size: 12px; text-transform: uppercase; letter-spacing: .06em; color: var(--muted); display: flex; gap: 8px; }
  .dur { text-transform: none; letter-spacing: 0; font-variant-numeric: tabular-nums; }
  audio { width: 100%; height: 36px; }
  .small { font-size: 12px; margin: 0; }
</style>
