<script lang="ts">
  import { api, errorText } from "./api";
  import type { TextChanges } from "../../bindings/dawgit/desktop/models";

  // A text file's changes line by line, between two versions: a version id,
  // "" for the file on disk now, "none" for no file (added or deleted). Shows
  // nothing for files that aren't text. `stamp` refetches "now".
  let { root, file, from, to, stamp = 0 }: {
    root: string; file: string; from: string; to: string; stamp?: number;
  } = $props();

  let d = $state<TextChanges | null>(null);
  let failed = $state("");
  let seq = 0;
  $effect(() => {
    const n = ++seq;
    stamp; // reload with it
    d = null;
    failed = "";
    api.TextDiff(root, file, from, to)
      .then((r) => { if (n === seq) d = r; })
      .catch((e) => { if (n === seq) failed = errorText(e); });
  });
</script>

{#if failed}
  <p class="muted small">Couldn't compare: {failed}</p>
{:else if d === null}
  <p class="muted small">Comparing…</p>
{:else if d.tooBig}
  <p class="muted small">Too big to compare line by line.</p>
{:else if d.text}
  {#if d.hunks.length === 0}
    <p class="muted small">No line changes (only line endings, if anything).</p>
  {:else}
    <div class="sum small">
      <span class="add">+{d.added}</span> <span class="del">−{d.removed}</span>
      <span class="faint">line{d.added + d.removed === 1 ? "" : "s"}</span>
    </div>
    <div class="diff mono">
      {#each d.hunks as h, i}
        {#if i > 0}<div class="gap" aria-hidden="true">⋯</div>{/if}
        {#each h.lines as l}
          <div class="row {l.kind}">
            <span class="no">{l.old || ""}</span><span class="no">{l.new || ""}</span>
            <span class="mark">{l.kind === "add" ? "+" : l.kind === "del" ? "−" : ""}</span>
            <span class="txt">{l.text}</span>
          </div>
        {/each}
      {/each}
    </div>
    {#if d.truncated}<p class="faint small">More changes than shown here.</p>{/if}
  {/if}
{/if}

<style>
  .small { font-size: 12px; }
  .sum { display: flex; gap: 8px; margin: 2px 0 6px; font-variant-numeric: tabular-nums; }
  .add { color: var(--add); }
  .del { color: var(--del); }
  .diff { background: var(--bg); border: 1px solid var(--line); border-radius: 6px; overflow: auto;
    max-height: 70vh; padding: 4px 0; line-height: 1.55; user-select: text; }
  .row { display: flex; min-width: max-content; }
  .row.add { background: rgba(111, 207, 127, .10); }
  .row.del { background: rgba(229, 103, 95, .11); }
  .no { width: 44px; flex: none; padding-right: 8px; text-align: right; color: var(--faint);
    font-variant-numeric: tabular-nums; user-select: none; }
  .mark { width: 16px; flex: none; text-align: center; user-select: none; }
  .row.add .mark { color: var(--add); }
  .row.del .mark { color: var(--del); }
  .txt { white-space: pre; padding-right: 12px; tab-size: 4; }
  .gap { color: var(--faint); padding: 0 0 0 100px; user-select: none; }
</style>
