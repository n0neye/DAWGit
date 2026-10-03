<script lang="ts">
  import { folderMoves } from "./moves";
  import { lineKind, type Change } from "./api";
  import FileIcon from "./FileIcon.svelte";
  import WeightSummary from "./WeightSummary.svelte";
  import { t } from "./i18n.svelte";

  // Changes as a tree of folders (open, each can be closed), what changed at
  // the end of each file's row, a set's track changes under it.
  let { changes, empty = "" }: { changes: Change[]; empty?: string } = $props();

  const sym: Record<string, string> = { added: "+", modified: "~", deleted: "−", untracked: "○", renamed: "M" };
  const statusName = (s: string) => ({ added: t("New"), modified: t("Changed"), deleted: t("Deleted"),
    untracked: t("No longer tracked"), renamed: t("Moved") } as Record<string, string>)[s];
  // Folders that moved, said once on the folder (see moves.ts).
  let moves = $derived(folderMoves(changes));
  const dirOf = (p: string) => p.slice(0, p.lastIndexOf("/") + 1);
  const fromLabel = (c: Change) => (dirOf(c.path) === dirOf(c.from) ? name(c.from) : c.from);
  const audio = /\.(wav|aiff?|mp3|flac|ogg|m4a)$/i;
  const kindOf = (p: string) => (/\.als$/i.test(p) ? "set" : audio.test(p) ? "audio" : "other");

  type Folder = { path: string; name: string; folders: Map<string, Folder>; files: Change[]; count: number };
  let tree = $derived.by(() => {
    const mk = (path: string, name: string): Folder => ({ path, name, folders: new Map(), files: [], count: 0 });
    const top = mk("", "");
    for (const c of changes) {
      const parts = c.path.split("/");
      let node = top;
      top.count++;
      for (let k = 0; k < parts.length - 1; k++) {
        if (!node.folders.has(parts[k])) node.folders.set(parts[k], mk(parts.slice(0, k + 1).join("/"), parts[k]));
        node = node.folders.get(parts[k])!;
        node.count++;
      }
      node.files.push(c);
    }
    return top;
  });
  type Row = { folder?: Folder; change?: Change; depth: number };
  let closed = $state<Record<string, boolean>>({});
  let rows = $derived.by(() => {
    const out: Row[] = [];
    const walk = (node: Folder, depth: number) => {
      for (const sub of [...node.folders.values()].sort((a, b) => a.name.localeCompare(b.name))) {
        out.push({ folder: sub, depth });
        if (!closed[sub.path]) walk(sub, depth + 1);
      }
      for (const c of [...node.files].sort((a, b) => a.path.localeCompare(b.path))) out.push({ change: c, depth });
    };
    walk(tree, 0);
    return out;
  });
  const name = (p: string) => p.slice(p.lastIndexOf("/") + 1);
</script>

{#if changes.length === 0}
  <p class="muted">{empty || t("No changes")}</p>
{:else}
  <ul class="changes">
    {#each rows as row (row.change ? row.change.path : "dir:" + row.folder!.path)}
      {#if row.folder}
        {@const d = row.folder}
        <li>
          <button class="row dir" style:padding-left="{row.depth * 14 + 2}px" onclick={() => (closed[d.path] = !closed[d.path])}>
            <svg class="chev" class:open={!closed[d.path]} viewBox="0 0 10 10" aria-hidden="true">
              <path d="M3 1.5 L7 5 L3 8.5" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" />
            </svg>
            <FileIcon kind="folder" open={!closed[d.path]} />
            <span class="fname">{d.name}</span>
            {#if moves.movedFrom(d.path)}<span class="from">← {moves.movedFrom(d.path)}/</span>{/if}
            {#if closed[d.path]}<span class="right"><span class="count">{d.count}</span></span>{/if}
          </button>
        </li>
      {:else}
        {@const c = row.change!}
        <li>
          <div class="row file {c.status}" style:padding-left="{row.depth * 14 + 20}px" title={c.path}>
            <FileIcon kind={kindOf(c.path)} faint={c.status === "deleted"} />
            <span class="fname">{name(c.path)}</span>
            {#if c.status === "renamed" && !moves.covered(c.path)}
              <span class="from" title={`Moved from ${c.from}${c.edited ? ", and changed" : ""}`}>← {fromLabel(c)}</span>
            {/if}
            <span class="right"><span class="sym" title={statusName(c.status)}>{sym[c.status] ?? "·"}</span></span>
          </div>
          {#if c.tracks?.length}
            <div class="wsum" style:margin-left="{row.depth * 14 + 42}px"><WeightSummary tracks={c.tracks} /></div>
          {/if}
          {#if c.details.length}
            <div class="details mono" style:margin-left="{row.depth * 14 + 42}px">
              {#each c.details as line}
                <div class={lineKind(line)} style:padding-left="{(line.length - line.trimStart().length) * 4 + 4}px">{line.trim()}</div>
              {/each}
            </div>
          {/if}
        </li>
      {/if}
    {/each}
  </ul>
{/if}

<style>
  .wsum { margin: 2px 0 2px; }
  .changes { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 1px; }
  .row { display: flex; align-items: center; gap: 6px; width: 100%; min-height: 26px; padding-top: 2px; padding-bottom: 2px;
    padding-right: 6px; border: none; border-radius: 5px; background: transparent; text-align: left; font-size: 13px; }
  button.row:hover { background: var(--panel); }
  .chev { width: 12px; height: 12px; flex: none; color: var(--muted); transition: transform .12s; }
  .chev.open { transform: rotate(90deg); }
  .dir .fname { color: var(--muted); }
  .fname { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .file.deleted .fname { text-decoration: line-through; color: var(--muted); }
  .right { margin-left: auto; display: flex; align-items: center; flex: none; }
  .count { font-size: 11px; padding: 0 6px; border-radius: 8px; background: #33363d; color: var(--mod); }
  .sym { width: 16px; height: 16px; border-radius: 4px; display: inline-flex; align-items: center; justify-content: center;
    font-size: 12px; font-weight: 700; line-height: 1; }
  .file.added .sym { color: var(--add); background: rgba(111, 207, 127, .16); }
  .file.deleted .sym { color: var(--del); background: rgba(229, 103, 95, .16); }
  .file.modified .sym { color: var(--mod); background: rgba(106, 176, 243, .16); }
  .file.untracked .sym { color: var(--muted); }
  .file.renamed .sym { color: var(--warn); background: rgba(232, 176, 75, .16); }
  .from { flex: 0 1000 auto; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    font-size: 11.5px; color: var(--faint); }
  .details {
    margin: 4px 0 6px; padding: 8px 10px; background: var(--bg); border: 1px solid var(--line);
    border-radius: 6px; line-height: 1.6; user-select: text;
  }
  .add { color: var(--add); }
  .del { color: var(--del); }
  .mod { color: var(--mod); }
</style>
