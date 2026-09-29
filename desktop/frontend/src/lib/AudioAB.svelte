<script lang="ts">
  // Two versions of a sample, one above the other. Starting one pauses the
  // other and picks up at the same position, to hear the difference.
  let { a, b }: {
    a: { label: string; src: string } | null; // newer
    b: { label: string; src: string } | null; // older
  } = $props();

  let top = $state<HTMLAudioElement>();
  let bottom = $state<HTMLAudioElement>();
  let failed = $state<Record<string, boolean>>({});

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
      <div class="row">
        <div class="label">{row.side.label}</div>
        {#if failed[row.side.src]}
          <p class="faint small">This file can't be played here.</p>
        {:else if row.el === "top"}
          <audio bind:this={top} controls preload="metadata" src={row.side.src}
            onplay={() => played(top, bottom)} onerror={() => (failed[a!.src] = true)}></audio>
        {:else}
          <audio bind:this={bottom} controls preload="metadata" src={row.side.src}
            onplay={() => played(bottom, top)} onerror={() => (failed[b!.src] = true)}></audio>
        {/if}
      </div>
    {/if}
  {/each}
  {#if a && b}<p class="faint small">Start one while the other plays to switch at the same position.</p>{/if}
</div>

<style>
  .ab { display: flex; flex-direction: column; gap: 14px; }
  .row { display: flex; flex-direction: column; gap: 6px; }
  .label { font-size: 12px; text-transform: uppercase; letter-spacing: .06em; color: var(--muted); }
  audio { width: 100%; }
  .small { font-size: 12px; margin: 0; }
</style>
