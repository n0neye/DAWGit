<script lang="ts">
  import type { Snippet } from "svelte";
  import { api, ago, errorText, fileURL, formatBytes, lineKind, type FileVersion, type ProjectFile, type State } from "./api";
  import { toast } from "./notify.svelte";
  import AudioAB from "./AudioAB.svelte";
  import FileIcon from "./FileIcon.svelte";
  import ConvertDialog from "./ConvertDialog.svelte";

  // Changes tab: files on the left, what changed on the right (the set's
  // tracks, or the sample to listen to, now and before). "All files" lists
  // the whole project folder. Each file has a menu (⋯ or right click).
  let { root, st, summary, ondiscard, ondiscardall, onrestore }: {
    root: string;
    st: State;
    summary: Snippet; // shown when no file is selected (tracks you changed)
    ondiscard: (path: string) => void;
    ondiscardall: () => void;
    onrestore: (path: string, version: string, label: string) => void; // one file from a version
  } = $props();

  // "All files" is remembered per project.
  let all = $state(false);
  $effect.pre(() => {
    const key = `dawgit.allFiles:${root}`;
    try { all = localStorage.getItem(key) === "1"; } catch { all = false; }
  });
  function rememberAll() {
    try { localStorage.setItem(`dawgit.allFiles:${root}`, all ? "1" : "0"); } catch { /* not remembered */ }
  }
  let files = $state<ProjectFile[]>([]);
  let selected = $state("");
  let mode = $state<"changes" | "history">("changes");
  let menu = $state<{ path: string; x: number; y: number } | null>(null);

  // history of the selected file
  let history = $state<FileVersion[] | null>(null);
  let picked = $state(""); // version id in the history
  let pickedDiff = $state<string[] | null>(null);

  let loadedAt = $state(0); // makes "now" previews fetch the file again
  let converting = $state(""); // sample in the Convert dialog
  function loadFiles(showAll: boolean) {
    return api.ProjectFiles(root, showAll).then((f) => { files = f ?? []; loadedAt = Date.now(); })
      .catch((e) => toast(errorText(e), "error"));
  }
  $effect(() => {
    st; // reload with the project's state
    loadFiles(all);
  });

  // Show a new file (e.g. a converted sample): open its folders, select it.
  async function reveal(p: string) {
    await loadFiles(all);
    const parts = p.split("/");
    for (let k = 1; k < parts.length; k++) open[parts.slice(0, k).join("/")] = true;
    select(p);
  }
  const nowURL = (p: string) => `${fileURL(root, p)}&t=${loadedAt}`;

  let current = $derived(files.find((f) => f.path === selected));
  let change = $derived(st.changes.find((c) => c.path === selected));
  // Live versions: "Ableton Live 12.3.1" -> "12.3". A set saved with another
  // Live than most sets in the project stands out.
  const liveShort = (c: string) => c.match(/(\d+\.\d+)/)?.[1] ?? "";
  let usualLive = $derived.by(() => {
    const count = new Map<string, number>();
    for (const f of files) if (f.live) count.set(liveShort(f.live), (count.get(liveShort(f.live)) ?? 0) + 1);
    return [...count.entries()].sort((a, b) => b[1] - a[1])[0]?.[0] ?? "";
  });
  let changedCount = $derived(files.filter((f) => f.status !== "unchanged" && f.status !== "ignored").length);

  // Folders as groups, files under them (flat list of rows).
  // The folder tree: at each level folders first, then files. Folders start
  // closed; a folder shows how many changed files it holds.
  type Folder = { path: string; name: string; folders: Map<string, Folder>; files: ProjectFile[];
    changed: number; tracked: boolean };
  let open = $state<Record<string, boolean>>({});

  let tree = $derived.by(() => {
    const mk = (path: string, name: string): Folder =>
      ({ path, name, folders: new Map(), files: [], changed: 0, tracked: false });
    const top = mk("", "");
    for (const f of files) {
      const parts = f.path.split("/");
      let node = top;
      const chain = [top];
      for (let k = 0; k < parts.length - 1; k++) {
        const path = parts.slice(0, k + 1).join("/");
        if (!node.folders.has(parts[k])) node.folders.set(parts[k], mk(path, parts[k]));
        node = node.folders.get(parts[k])!;
        chain.push(node);
      }
      node.files.push(f);
      for (const n of chain) {
        if (f.status !== "unchanged" && f.status !== "ignored") n.changed++;
        if (f.status !== "ignored") n.tracked = true;
      }
    }
    return top;
  });

  type Row = { folder?: Folder; file?: ProjectFile; depth: number };
  let rows = $derived.by(() => {
    const out: Row[] = [];
    const walk = (node: Folder, depth: number) => {
      for (const sub of [...node.folders.values()].sort((a, b) => a.name.localeCompare(b.name))) {
        out.push({ folder: sub, depth });
        if (open[sub.path]) walk(sub, depth + 1);
      }
      for (const f of [...node.files].sort((a, b) => a.path.localeCompare(b.path))) out.push({ file: f, depth });
    };
    walk(tree, 0);
    return out;
  });

  function select(p: string) {
    selected = p;
    mode = "changes";
  }

  async function showHistory(p: string) {
    menu = null;
    selected = p;
    mode = "history";
    history = null;
    picked = "";
    pickedDiff = null;
    try {
      history = await api.FileHistory(root, p);
      if (history?.length) pick(history[0].version.id);
    } catch (e) {
      toast(errorText(e), "error");
      history = [];
    }
  }

  // The version before `id` in this file's history (to compare with).
  const before = (id: string) => {
    const i = history?.findIndex((h) => h.version.id === id) ?? -1;
    return i >= 0 ? history![i + 1] : undefined;
  };

  async function pick(id: string) {
    picked = id;
    pickedDiff = null;
    if (!current || current.kind !== "set") return;
    try {
      pickedDiff = await api.FileDiff(root, selected, before(id)?.version.id ?? "", id);
    } catch (e) {
      toast(errorText(e), "error");
    }
  }

  function openMenu(e: MouseEvent, p: string) {
    e.preventDefault();
    menu = { path: p, x: e.clientX, y: e.clientY };
  }

  const sym: Record<string, string> = { added: "+", modified: "~", deleted: "−", unchanged: "", ignored: "" };
  const name = (p: string) => p.slice(p.lastIndexOf("/") + 1);
  const canDiscard = (f: ProjectFile | undefined) => !!f && (f.status === "added" || f.status === "modified" || f.status === "deleted");
</script>

<svelte:window onclick={(e) => { if (menu && !(e.target as HTMLElement).closest(".ctx")) menu = null; }}
  onkeydown={(e) => { if (e.key === "Escape") menu = null; }} />

<div class="panel">
  <aside class="files">
    <div class="files-h">
      <span>{all ? "All files" : `Changed files${changedCount ? ` (${changedCount})` : ""}`}</span>
      {#if changedCount}
        <button class="ghost discard-all" onclick={ondiscardall} title="Drop all uncommitted changes">Discard all…</button>
      {/if}
      <label class="all" title="List every file in the project folder">
        <input type="checkbox" class="switch" role="switch" bind:checked={all} onchange={rememberAll} /> All files
      </label>
    </div>
    {#if files.length === 0}
      <p class="muted empty">{all ? "The project folder is empty." : "No uncommitted changes. Work in Live and press Ctrl+S — your changes show up here."}</p>
    {:else}
      <ul>
        {#each rows as row (row.file ? row.file.path : "dir:" + row.folder!.path)}
          {#if row.folder}
            {@const d = row.folder}
            <li>
              <button class="file dir" class:untracked={!d.tracked} class:changed={d.changed > 0} style:padding-left="{8 + row.depth * 14}px"
                onclick={() => (open[d.path] = !open[d.path])} title={d.tracked ? d.path : `${d.path} — not tracked`}>
                <svg class="chev" class:open={open[d.path]} viewBox="0 0 10 10" aria-hidden="true">
                  <path d="M3 1.5 L7 5 L3 8.5" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" />
                </svg>
                <FileIcon kind="folder" open={open[d.path]} faint={!d.tracked} />
                <span class="fname">{d.name}</span>
                {#if d.changed && !open[d.path]}<span class="count" title="Changed files inside">{d.changed}</span>{/if}
                {#if !d.tracked}<span class="tag dir-tag">not tracked</span>{/if}
              </button>
            </li>
          {:else}
            {@const f = row.file!}
            <li>
              <button class="file {f.status}" class:on={f.path === selected} style:padding-left="{8 + row.depth * 14}px"
                onclick={() => select(f.path)} oncontextmenu={(e) => openMenu(e, f.path)} title={f.path}>
                <span class="sym">{sym[f.status]}</span>
                <FileIcon kind={f.kind} faint={f.status === "ignored" || f.status === "deleted"} />
                <span class="fname">{name(f.path)}</span>
                {#if f.live}
                  {@const v = liveShort(f.live)}
                  <span class="live" class:odd={usualLive && v !== usualLive}
                    title={`Saved with ${f.live}${usualLive && v !== usualLive ? ` — most sets here use Live ${usualLive}` : ""}`}>{v}</span>
                {/if}
                {#if f.status === "ignored"}<span class="tag">not tracked</span>{/if}
              </button>
              <button class="ghost more" title="More" onclick={(e) => { e.stopPropagation(); openMenu(e, f.path); }}>⋯</button>
            </li>
          {/if}
        {/each}
      </ul>
    {/if}
  </aside>

  <section class="detail">
    {#if !selected || !current}
      {@render summary()}
    {:else}
      <div class="detail-h">
        <div class="title">
          <div class="dname"><FileIcon kind={current.kind} /> {name(selected)}</div>
          <div class="faint small mono">{selected}</div>
          {#if current.live}<div class="faint small">Saved with {current.live}</div>{/if}
        </div>
        <div class="modes">
          <button class:on={mode === "changes"} onclick={() => (mode = "changes")}>Changes</button>
          <button class:on={mode === "history"} onclick={() => showHistory(selected)}
            disabled={current.status === "ignored"}>History</button>
        </div>
      </div>

      {#if mode === "changes"}
        {#if current.status === "ignored"}
          <p class="muted">DAWGit doesn't keep this file in versions (Live's backups and analysis files are left out).</p>
        {:else if current.status === "unchanged"}
          <p class="muted">No changes since the version you're on. {formatBytes(current.size)}</p>
        {:else}
          <p class="muted">{current.status === "added" ? "New file" : current.status === "deleted" ? "Deleted" : "Changed"}
            since the version you're on{current.size ? ` · ${formatBytes(current.size)}` : ""}.</p>
        {/if}

        {#if current.kind === "audio" && current.status !== "ignored"}
          <AudioAB
            a={current.status !== "deleted" ? { label: current.status === "unchanged" ? "In the project" : "Now (not committed)", src: nowURL(selected) } : null}
            b={st.head && (current.status === "modified" || current.status === "deleted")
              ? { label: "In the version you're on", src: fileURL(root, selected, st.head) } : null} />
        {:else if change?.details.length}
          <div class="lines mono">
            {#each change.details as line}
              <div class={lineKind(line)} style:padding-left="{(line.length - line.trimStart().length) * 4 + 4}px">{line.trim()}</div>
            {/each}
          </div>
        {/if}
      {:else}
        {#if history === null}
          <p class="muted">Loading…</p>
        {:else if history.length === 0}
          <p class="muted">No committed versions of this file yet.</p>
        {:else}
          <ul class="versions">
            {#each history as h (h.version.id)}
              <li>
                <button class:on={h.version.id === picked} onclick={() => pick(h.version.id)}>
                  <span class="vsym {h.status}">{sym[h.status]}</span>
                  <span class="vmsg">{h.version.message || "(no description)"}</span>
                  <span class="faint">{h.version.author} · {ago(h.version.time)}</span>
                </button>
              </li>
            {/each}
          </ul>
          {#if picked}
            {@const h = history.find((x) => x.version.id === picked)!}
            {@const prev = before(picked)}
            <div class="picked">
              {#if h.status !== "deleted"}
                <div class="restore">
                  <button onclick={() => onrestore(selected, h.version.id, h.version.message || h.version.short)}
                    title="Put this file back as it was in this version; the rest of the project stays">Restore this version</button>
                  <span class="faint small">Only this file changes; commit it when you're happy.</span>
                </div>
              {/if}
              {#if current.kind === "audio"}
                <AudioAB
                  a={h.status !== "deleted" ? { label: `“${h.version.message || h.version.short}”`, src: fileURL(root, selected, h.version.id) } : null}
                  b={prev && prev.status !== "deleted" ? { label: `Before: “${prev.version.message || prev.version.short}”`, src: fileURL(root, selected, prev.version.id) } : null} />
              {:else if current.kind === "set"}
                {#if pickedDiff === null}
                  <p class="muted">Comparing…</p>
                {:else if pickedDiff.length === 0}
                  <p class="muted">{h.status === "added" ? "The set was added in this version." : "No track changes in this version."}</p>
                {:else}
                  <div class="lines mono">
                    {#each pickedDiff as line}
                      <div class={lineKind(line)} style:padding-left="{(line.length - line.trimStart().length) * 4 + 4}px">{line.trim()}</div>
                    {/each}
                  </div>
                {/if}
              {:else}
                <p class="muted">{h.status === "added" ? "Added" : h.status === "deleted" ? "Deleted" : "Changed"} in this version.</p>
              {/if}
            </div>
          {/if}
        {/if}
      {/if}
    {/if}
  </section>
</div>

{#if menu}
  {@const f = files.find((x) => x.path === menu!.path)}
  <div class="ctx" role="menu" style:left="{Math.min(menu.x, window.innerWidth - 240)}px" style:top="{Math.min(menu.y, window.innerHeight - 200)}px">
    <button class="item" disabled={f?.status === "ignored"} onclick={() => showHistory(menu!.path)}>View file history</button>
    {#if canDiscard(f)}
      <button class="item danger-text" onclick={() => { const p = menu!.path; menu = null; ondiscard(p); }}>Discard changes…</button>
    {/if}
    <div class="sep"></div>
    {#if f && f.status !== "deleted"}
      <button class="item" onclick={() => { const p = menu!.path; menu = null; api.ShowFile(root, p).catch((e) => toast(errorText(e), "error")); }}>Show in Explorer</button>
      {#if f.kind === "set"}
        <button class="item" onclick={() => { const p = menu!.path; menu = null; api.OpenInLive(root, p); }}>Open in Live</button>
      {:else if f.kind === "audio"}
        <button class="item" onclick={() => { const p = menu!.path; menu = null; api.OpenInLive(root, p); }}>Open in default player</button>
        <button class="item" onclick={() => { converting = menu!.path; menu = null; }}>Convert…</button>
      {/if}
    {/if}
  </div>
{/if}

{#if converting}
  <ConvertDialog {root} file={converting} onclose={() => (converting = "")}
    ondone={(p) => { converting = ""; toast(`Converted to ${p.slice(p.lastIndexOf("/") + 1)}`, "ok"); reveal(p); }} />
{/if}

<style>
  .panel { display: grid; grid-template-columns: minmax(240px, 34%) 1fr; height: 100%; min-height: 0; }
  .files { border-right: 1px solid var(--line); overflow: auto; min-height: 0; padding: 10px 8px 16px 0; }
  .files-h { display: flex; align-items: center; gap: 8px; padding: 0 4px 8px 8px; font-size: 12px;
    text-transform: uppercase; letter-spacing: .06em; color: var(--muted); }
  .files-h span { flex: 1; }
  .all { display: flex; align-items: center; gap: 5px; margin: 0; text-transform: none; letter-spacing: 0; cursor: pointer; }
  /* iOS-style switch */
  .switch { appearance: none; position: relative; width: 26px; height: 15px; margin: 0; flex: none; cursor: pointer;
    border: none; padding: 0; border-radius: 8px; background: #3a3d45; transition: background .15s; }
  .switch::after { content: ""; position: absolute; top: 2px; left: 2px; width: 11px; height: 11px; border-radius: 50%;
    background: #d8dae0; transition: transform .15s; }
  .switch:checked { background: var(--accent); }
  .switch:checked::after { transform: translateX(11px); background: #fff; }
  .switch:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
  .empty { padding: 0 8px; font-size: 13px; }
  ul { list-style: none; margin: 0; padding: 0; }
  li { position: relative; display: flex; }
  .chev { width: 12px; height: 12px; flex: none; color: var(--muted); transition: transform .12s; }
  .chev.open { transform: rotate(90deg); }
  /* Folders read like files: bright when they hold changes, muted otherwise,
     faint when nothing inside is tracked. */
  .dir .fname { color: var(--muted); }
  .dir.changed .fname { color: var(--text); }
  .dir.untracked .fname, .dir.untracked .chev { color: var(--faint); }
  .count { font-size: 11px; padding: 0 6px; border-radius: 8px; background: #33363d; color: var(--mod); }
  .file { flex: 1; min-width: 0; display: flex; align-items: center; gap: 6px; border: none; background: transparent;
    padding: 5px 30px 5px 8px; border-radius: 6px; text-align: left; font-size: 13.5px; }
  .file:hover { background: var(--panel); }
  .file.on { background: var(--panel-2); }
  .sym { width: 12px; text-align: center; font-weight: 700; }
  .file.added .sym { color: var(--add); }
  .file.deleted .sym { color: var(--del); }
  .file.modified .sym { color: var(--mod); }
  .file.deleted .fname { text-decoration: line-through; color: var(--muted); }
  .file.ignored .fname, .file.unchanged .fname { color: var(--muted); }
  .file.ignored .fname { color: var(--faint); }
  .fname { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .live { font-size: 10.5px; padding: 0 5px; border-radius: 7px; background: #33363d; color: var(--muted);
    font-variant-numeric: tabular-nums; flex: none; }
  .live.odd { background: var(--warn-bg); color: var(--warn); }
  /* Untracked shows as a faint name; the label only on hover, laid over the
     end of the name so rows don't shift. */
  .tag { position: absolute; right: 30px; top: 50%; transform: translateY(-50%); display: none; pointer-events: none;
    font-size: 10.5px; color: var(--muted); border: 1px solid var(--line); border-radius: 8px; padding: 0 5px;
    background: var(--panel); box-shadow: -10px 0 8px var(--panel); white-space: nowrap; }
  .tag.dir-tag { right: 8px; }
  .file:hover .tag, li:hover .tag { display: block; }
  .file.on .tag { background: var(--panel-2); box-shadow: -10px 0 8px var(--panel-2); }
  .more { position: absolute; right: 4px; top: 50%; transform: translateY(-50%); visibility: hidden; padding: 0 6px; }
  li:hover .more { visibility: visible; }

  .detail { overflow: auto; min-height: 0; padding: 12px 4px 16px 20px; }
  .detail-h { display: flex; align-items: flex-start; gap: 12px; margin-bottom: 10px; }
  .title { flex: 1; min-width: 0; }
  .dname { display: flex; align-items: center; gap: 6px; font-weight: 650; font-size: 15px; }
  .small { font-size: 12px; }
  .modes { display: flex; }
  .modes button { padding: 4px 10px; font-size: 12.5px; border-radius: 0; }
  .modes button:first-child { border-radius: 6px 0 0 6px; }
  .modes button:last-child { border-radius: 0 6px 6px 0; margin-left: -1px; }
  .modes button.on { background: var(--accent); color: var(--accent-ink); border-color: var(--accent); }
  .lines { padding: 8px 10px; background: var(--bg); border: 1px solid var(--line); border-radius: 6px; line-height: 1.6; user-select: text; }
  .add { color: var(--add); }
  .del { color: var(--del); }
  .mod { color: var(--mod); }
  .versions { display: flex; flex-direction: column; gap: 2px; margin-bottom: 14px; }
  .versions button { width: 100%; display: flex; align-items: center; gap: 8px; border: none; background: transparent;
    padding: 6px 8px; border-radius: 6px; text-align: left; }
  .versions button:hover { background: var(--panel); }
  .versions button.on { background: var(--panel-2); }
  .vsym { width: 12px; text-align: center; font-weight: 700; }
  .vsym.added { color: var(--add); }
  .vsym.deleted { color: var(--del); }
  .vsym.modified { color: var(--mod); }
  .vmsg { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .picked { border-top: 1px solid var(--line); padding-top: 14px; }
  .restore { display: flex; align-items: center; gap: 10px; margin-bottom: 14px; }
  .restore button { padding: 5px 12px; font-size: 13px; }
  .discard-all { padding: 0 6px; font-size: 11.5px; text-transform: none; letter-spacing: 0; color: var(--muted); }

  .ctx { position: fixed; z-index: 40; min-width: 210px; padding: 6px; background: var(--panel-2);
    border: 1px solid var(--line); border-radius: 8px; box-shadow: 0 12px 30px rgba(0, 0, 0, .45); }
  .ctx .item { display: block; width: 100%; border: none; background: transparent; padding: 6px 8px; text-align: left; }
  .ctx .item:hover:not(:disabled) { background: #33363d; }
  .danger-text { color: var(--danger); }
  .sep { height: 1px; background: var(--line); margin: 6px 0; }
</style>
