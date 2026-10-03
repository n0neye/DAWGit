<script lang="ts">
  import { tn } from "./i18n.svelte";
  import { byWeight, isSmall, names, weightHint, weightName, type TrackWeight } from "./weights";

  // What changed in a set, by how much it matters: "Arrangement Drums,
  // Bass · Sound Vital · Tidying 3 tracks". Small changes say how many,
  // not which.
  let { tracks }: { tracks: TrackWeight[] } = $props();
  let groups = $derived(byWeight(tracks));
</script>

{#if groups.length}
  <span class="summary">
    {#each groups as [w, list]}
      <span class="part" class:small={isSmall(w)} title={weightHint(w)}>
        <span class="pill {w}">{weightName(w)}</span>
        {isSmall(w) ? tn(list.length, "{n} track", "{n} tracks") : names(list)}
      </span>
    {/each}
  </span>
{/if}

<style>
  .summary { display: inline-flex; flex-wrap: wrap; gap: 4px 12px; align-items: center; font-size: 12px; }
  .part { display: inline-flex; align-items: center; gap: 5px; color: var(--text); }
  .part.small { color: var(--faint); }
  .pill { font-size: 10.5px; font-weight: 600; padding: 0 6px; border-radius: 7px; line-height: 15px; }
  .pill.arrangement { background: rgba(240, 113, 120, .2); color: #f4a3a8; }
  .pill.sound { background: rgba(199, 146, 234, .2); color: #d9b5f1; }
  .pill.mix { background: rgba(106, 176, 243, .18); color: #9ccbf7; }
  .pill.tidy, .pill.noise { background: rgba(154, 158, 168, .15); color: var(--muted); }
</style>
