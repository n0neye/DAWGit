<script lang="ts">
  import { api, errorText } from "./api";
  import { view } from "./compare.svelte";
  import type { TextChanges, TextContent } from "../../bindings/dawgit/desktop/models";

  // A text file: the whole of one version (preview), or, when comparing
  // (view.compare), its changes line by line from the version before —
  // around the changes, or the whole file. A version is a version id, "" for
  // the file on disk now, "none" for no file (added or deleted). `stamp`
  // refetches "now". Files that aren't text show a short note.
  let { root, file, from, to, stamp = 0 }: {
    root: string; file: string; from: string; to: string; stamp?: number;
  } = $props();

  const WHOLE = "dawgit.wholeFile";
  let whole = $state((() => { try { return localStorage.getItem(WHOLE) === "1"; } catch { return false; } })());
  function setWhole(on: boolean) {
    whole = on;
    try { localStorage.setItem(WHOLE, on ? "1" : "0"); } catch { /* not remembered */ }
  }

  // Comparing needs a version before; the one shown alone is the newer one.
  let canCompare = $derived(from !== to && !(from === "none" && to === "none"));
  let comparing = $derived(view.compare && canCompare);
  let shown = $derived(to === "none" ? from : to);

  let diff = $state<TextChanges | null>(null);
  let content = $state<TextContent | null>(null);
  let failed = $state("");
  let seq = 0;
  $effect(() => {
    const n = ++seq;
    stamp; // reload with it
    diff = null;
    content = null;
    failed = "";
    const fail = (e: unknown) => { if (n === seq) failed = errorText(e); };
    if (comparing) {
      api.TextDiff(root, file, from, to, whole).then((r) => {
        if (n !== seq) return;
        diff = r;
        // Nothing changed: the file itself, then.
        if (r && r.text && r.hunks.length === 0) {
          api.TextFile(root, file, shown).then((c) => { if (n === seq) content = c; }).catch(fail);
        }
      }).catch(fail);
    } else {
      api.TextFile(root, file, shown).then((c) => { if (n === seq) content = c; }).catch(fail);
    }
  });
</script>

{#snippet lines(c: TextContent)}
  {#if c.lines.length === 0}
    <p class="muted small">An empty file.</p>
  {:else}
    <div class="code mono">
      {#each c.lines as l, i}
        <div class="row"><span class="no">{i + 1}</span><span class="txt">{l}</span></div>
      {/each}
    </div>
    {#if c.truncated}<p class="faint small">The file goes on: only its first {c.lines.length} lines are shown.</p>{/if}
  {/if}
{/snippet}

{#if failed}
  <p class="muted small">Couldn't read it: {failed}</p>
{:else if comparing}
  {#if diff === null}
    <p class="muted small">Comparing…</p>
  {:else if diff.tooBig}
    <p class="muted small">Too big to compare line by line.</p>
  {:else if !diff.text}
    <p class="muted small">Not a text file: there are no lines to compare.</p>
  {:else}
    <div class="bar small">
      {#if diff.hunks.length}
        <span class="add">+{diff.added}</span> <span class="del">−{diff.removed}</span>
        <span class="faint">line{diff.added + diff.removed === 1 ? "" : "s"}</span>
      {:else}
        <span class="faint">No line changes.</span>
      {/if}
      <label class="whole"><input type="checkbox" checked={whole}
        onchange={(e) => setWhole((e.currentTarget as HTMLInputElement).checked)} /> Whole file</label>
    </div>
    {#if diff.hunks.length}
      <div class="code mono">
        {#each diff.hunks as h, i}
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
      {#if diff.truncated}<p class="faint small">More changes than shown here.</p>{/if}
    {:else if content}
      {@render lines(content)}
    {/if}
  {/if}
{:else if content === null}
  <p class="muted small">Reading…</p>
{:else if content.tooBig}
  <p class="muted small">Too big to show here.</p>
{:else if !content.text}
  <p class="muted small">No preview for this kind of file.</p>
{:else}
  {@render lines(content)}
{/if}

<style>
  .small { font-size: 12px; }
  .bar { display: flex; align-items: center; gap: 8px; margin: 2px 0 6px; font-variant-numeric: tabular-nums; }
  .whole { margin: 0 0 0 auto; display: flex; align-items: center; gap: 5px; color: var(--muted); cursor: pointer; }
  .whole input { width: auto; margin: 0; padding: 0; }
  .add { color: var(--add); }
  .del { color: var(--del); }
  .code { background: var(--bg); border: 1px solid var(--line); border-radius: 6px; overflow: auto;
    max-height: 70vh; padding: 4px 0; line-height: 1.55; user-select: text; }
  .row { display: flex; min-width: max-content; }
  .row.add { background: rgba(111, 207, 127, .10); }
  .row.del { background: rgba(229, 103, 95, .11); }
  .no { width: 44px; flex: none; padding-right: 8px; text-align: right; color: var(--faint);
    font-variant-numeric: tabular-nums; user-select: none; }
  .mark { width: 16px; flex: none; text-align: center; user-select: none; }
  .row.add .mark { color: var(--add); }
  .row.del .mark { color: var(--del); }
  .txt { white-space: pre; padding: 0 12px 0 6px; tab-size: 4; }
  .gap { color: var(--faint); padding: 0 0 0 100px; user-select: none; }
</style>
