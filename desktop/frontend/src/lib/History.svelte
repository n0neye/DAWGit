<script lang="ts">
  import { ago, type Version } from "./api";
  import { layout } from "./graph";

  let { versions, head, incoming }: { versions: Version[]; head: string; incoming: Set<string> } = $props();

  const ROW = 40, LANE = 16, PAD = 12;
  let rows = $derived(layout(versions));
  let graphWidth = $derived(PAD * 2 + LANE * Math.max(1, ...rows.map((r) => r.width)));
  const x = (lane: number) => PAD + lane * LANE;
</script>

{#if versions.length === 0}
  <p class="muted">No versions yet. Commit your first version from the Changes tab.</p>
{:else}
  <div class="history">
    <svg width={graphWidth} height={versions.length * ROW} class="graph">
      {#each rows as r, i}
        {#each r.down as s}
          <path d="M {x(s.from)} {i * ROW + ROW / 2} C {x(s.from)} {i * ROW + ROW}, {x(s.to)} {i * ROW + ROW}, {x(s.to)} {(i + 1) * ROW + ROW / 2}"
            stroke="var(--lane-{s.color})" stroke-width="2" fill="none" />
        {/each}
      {/each}
      {#each rows as r, i}
        <circle cx={x(r.lane)} cy={i * ROW + ROW / 2} r={versions[i].id === head ? 6 : 4.5}
          fill={incoming.has(versions[i].id) ? "var(--bg)" : `var(--lane-${r.color})`}
          stroke="var(--lane-{r.color})" stroke-width="2" />
      {/each}
    </svg>
    <ul style:padding-left="{graphWidth}px">
      {#each versions as v, i (v.id)}
        <li style:height="{ROW}px" class:incoming={incoming.has(v.id)}>
          <span class="msg">
            {v.message || "(no description)"}
            {#each v.branches as b}<span class="tag">{b}</span>{/each}
            {#if v.id === head}<span class="tag here">you are here</span>{/if}
            {#if incoming.has(v.id)}<span class="tag new">new</span>{/if}
          </span>
          <span class="who">{v.author}</span>
          <span class="when faint">{ago(v.time)}</span>
          <span class="id mono faint">{v.short}</span>
        </li>
      {/each}
    </ul>
  </div>
{/if}

<style>
  .history { position: relative; }
  .graph { position: absolute; left: 0; top: 0; }
  ul { list-style: none; margin: 0; }
  li { display: flex; align-items: center; gap: 12px; border-bottom: 1px solid #25272c; }
  li.incoming .msg { color: var(--muted); }
  .msg { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .who { width: 90px; color: var(--muted); }
  .when { width: 110px; text-align: right; }
  .id { width: 80px; text-align: right; }
  .tag {
    display: inline-block; margin-left: 8px; padding: 0 7px; border-radius: 10px; font-size: 11.5px;
    background: #2b3a45; color: #9fd0f5; vertical-align: 1px;
  }
  .tag.here { background: #1f3b35; color: var(--accent); }
  .tag.new { background: var(--warn-bg); color: var(--warn); }
</style>
