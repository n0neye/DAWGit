<script lang="ts">
  import { folderMoves } from "./moves";
  import type { Snippet } from "svelte";
  import { api, ago, errorText, formatBytes, type FileVersion, type ProjectFile, type State } from "./api";
  import type { IgnoreOption } from "../../bindings/dawgit/desktop/models";
  import { toast } from "./notify.svelte";
  import FileIcon from "./FileIcon.svelte";
  import ConvertDialog from "./ConvertDialog.svelte";
  import { fileView, setFileMode, type FileMode } from "./viewmode.svelte";
  import { viewerFor, type Side } from "./viewers";

  // Changes tab: files on the left; on the right the selected file, as it is
  // (Preview), against the version you're on (Changes) or through its
  // versions (History), shown by its kind's viewer (viewers/). "All files"
  // lists the whole project folder. Each file has a menu (⋯ or right click).
  let { root, st, summary, commitBox, excluded = $bindable({}), ondiscard, ondiscardall, ondiscardsome, onrestore, onrules }: {
    root: string;
    st: State;
    excluded?: Record<string, boolean>; // changes unticked: left out of the next commit
    summary: Snippet; // shown when no file is selected (tracks you changed)
    commitBox?: Snippet; // under the files: the message and the Commit button
    ondiscard: (path: string) => void;
    ondiscardall: () => void;
    ondiscardsome: (paths: string[]) => void; // the ticked changes (not all of them)
    onrestore: (path: string, version: string, label: string, source: string) => void; // one file from a version (source: its path then)
    onrules?: () => void; // .dawgit.yaml changed (a file or folder left out)
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
  // The menu of a file or folder (right click, or ⋯), with the ways to
  // leave it out of versions.
  let menu = $state<{ path: string; x: number; y: number; dir: boolean; ignore: IgnoreOption[] } | null>(null);
  let ignoreOpen = $state(false); // the Ignore submenu

  // history of the selected file
  let history = $state<FileVersion[] | null>(null);
  let picked = $state(""); // version id in the history

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

  let current = $derived(files.find((f) => f.path === selected));
  // Where the selected file is in the version you're on (moved: elsewhere).
  let before_ = $derived(current?.status === "renamed" && current.from ? current.from : selected);
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
    changed: number; changedSize: number; tracked: boolean };
  // Folders start open in the list of changes (a tree of them), closed in
  // All files; either way they remember being opened or closed.
  let open = $state<Record<string, boolean>>({});
  const isOpen = (p: string) => open[p] ?? !all;
  const toggleFolder = (p: string) => (open[p] = !isOpen(p));

  let tree = $derived.by(() => {
    const mk = (path: string, name: string): Folder =>
      ({ path, name, folders: new Map(), files: [], changed: 0, changedSize: 0, tracked: false });
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
        if (f.status !== "unchanged" && f.status !== "ignored") { n.changed++; n.changedSize += f.size; }
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
        if (isOpen(sub.path)) walk(sub, depth + 1);
      }
      for (const f of [...node.files].sort((a, b) => a.path.localeCompare(b.path))) out.push({ file: f, depth });
    };
    walk(tree, 0);
    return out;
  });

  // Only the rows in view are drawn (a folder can hold thousands of files);
  // rows have a fixed height.
  const ROW = 30;
  let scroller = $state<HTMLElement>();
  let list = $state<HTMLElement>();
  let scrollTop = $state(0);
  let viewH = $state(800);
  let listOffset = $state(0); // where the list starts in the scrolled panel
  function onScroll() {
    if (!scroller || !list) return;
    scrollTop = scroller.scrollTop;
    listOffset = list.getBoundingClientRect().top - scroller.getBoundingClientRect().top + scroller.scrollTop;
  }
  $effect(() => {
    rows; // the list may have moved (e.g. the empty note went away)
    onScroll();
  });
  let win = $derived.by(() => {
    const top = scrollTop - listOffset;
    const from = Math.max(0, Math.floor(top / ROW) - 15);
    return { from, to: Math.min(rows.length, Math.ceil((top + viewH) / ROW) + 15) };
  });

  function select(p: string) {
    selected = p;
  }

  // History: the selected file's versions, read when it is shown.
  let historyOf = "";
  $effect(() => {
    if (fileView.mode === "history" && selected && selected !== historyOf) loadHistory(selected);
    if (fileView.mode !== "history") historyOf = "";
  });
  async function loadHistory(p: string) {
    historyOf = p;
    history = null;
    picked = "";
    try {
      const h = await api.FileHistory(root, p);
      if (historyOf !== p) return;
      history = h;
      if (h?.length) picked = h[0].version.id;
    } catch (e) {
      toast(errorText(e), "error");
      history = [];
    }
  }
  // From the file's menu.
  function showHistory(p: string) {
    menu = null;
    selected = p;
    setFileMode("history");
  }

  // The version before `id` in this file's history (to compare with).
  const before = (id: string) => {
    const i = history?.findIndex((h) => h.version.id === id) ?? -1;
    return i >= 0 ? history![i + 1] : undefined;
  };

  const pick = (id: string) => (picked = id);

  function openMenu(e: MouseEvent, p: string, dir = false) {
    e.preventDefault();
    ignoreOpen = false;
    const m = { path: p, x: e.clientX, y: e.clientY, dir, ignore: [] as IgnoreOption[] };
    menu = m;
    api.IgnoreOptions(p, dir).then((o) => { if (menu?.path === p) menu = { ...menu, ignore: o ?? [] }; }).catch(() => {});
  }

  async function ignore(pattern: string) {
    menu = null;
    try {
      await api.AddIgnoreRule(root, pattern);
      toast(`Left out of versions: ${pattern} — a rule in .dawgit.yaml; commit it to share it with the team`, "ok", 7000);
      await loadFiles(all);
      onrules?.();
    } catch (e) {
      toast(errorText(e), "error", 9000);
    }
  }

  function openFile(p: string) {
    menu = null;
    api.OpenInLive(root, p).catch((e) => toast(errorText(e), "error"));
  }

  // Ticking: each change, or a folder's changes at once.
  const changedPaths = $derived(st.changes.map((c) => c.path));
  const isChange = (f: ProjectFile) => f.status !== "unchanged" && f.status !== "ignored";
  const inside = (dir: string) => changedPaths.filter((p) => p.startsWith(dir + "/"));
  function tick(paths: string[], on: boolean) {
    const next = { ...excluded };
    for (const p of paths) {
      if (on) delete next[p];
      else next[p] = true;
    }
    excluded = next;
  }
  // The box over the list: every change ticked, none, or some.
  let allState = $derived.by((): "on" | "off" | "some" => {
    const out = changedPaths.filter((p) => excluded[p]).length;
    return out === 0 ? "on" : out === changedPaths.length ? "off" : "some";
  });
  // A folder's box: ticked, unticked, or some (indeterminate).
  function folderState(dir: string): "on" | "off" | "some" {
    const ps = inside(dir);
    const out = ps.filter((p) => excluded[p]).length;
    return out === 0 ? "on" : out === ps.length ? "off" : "some";
  }

  // What the selected file's viewer is given in Preview and Changes: the
  // state looked at (a), the one it is compared with (b), and whether to
  // show the differences. History gives each version and the one before.
  function sides(mode: FileMode, f: ProjectFile): { a: Side | null; b: Side | null; compare: boolean } {
    const now: Side = { path: f.path, version: "", label: f.status === "unchanged" ? "In the project" : "Now (not committed)" };
    const head: Side | null = st.head ? { path: before_, version: st.head, label: "In the version you're on" } : null;
    if (mode === "preview" || f.status === "unchanged") {
      // a deleted file: as it was
      return f.status === "deleted" ? { a: head, b: null, compare: false } : { a: now, b: null, compare: false };
    }
    const hadOne = f.status === "modified" || f.status === "deleted" || f.status === "renamed";
    return { a: f.status === "deleted" ? null : now, b: hadOne ? head : null, compare: true };
  }
  const versionLabel = (v: FileVersion) => `“${v.version.message || v.version.short}”`;

  const sym: Record<string, string> = { added: "+", modified: "~", deleted: "−", untracked: "○", renamed: "M", unchanged: "", ignored: "" };
  const statusName: Record<string, string> = { added: "New", modified: "Changed", deleted: "Deleted", renamed: "Moved",
    untracked: "No longer tracked: the rules leave it out now" };
  // Folders that moved, said once on the folder (see moves.ts).
  let moves = $derived(folderMoves(files));
  // Where a moved file was, said briefly: its old name in the same folder,
  // or its old folder.
  function fromLabel(f: { path: string; from: string }): string {
    const dir = (p: string) => p.slice(0, p.lastIndexOf("/") + 1);
    return dir(f.path) === dir(f.from) ? name(f.from) : f.from;
  }
  const name = (p: string) => p.slice(p.lastIndexOf("/") + 1);
  const canDiscard = (f: ProjectFile | undefined) => !!f && ["added", "modified", "deleted", "renamed"].includes(f.status);
</script>

<svelte:window onclick={(e) => { if (menu && !(e.target as HTMLElement).closest(".ctx")) menu = null; }}
  onkeydown={(e) => { if (e.key === "Escape") menu = null; }} />

<div class="panel">
  <div class="side">
  <aside class="files" bind:this={scroller} bind:clientHeight={viewH} onscroll={onScroll}>
    <!-- the list's header, kept at the top: the box to tick all, the title, how many and how big -->
    <!-- one line when there's room, else two: [box] title · how many, how big · revert · All files -->
    <div class="head">
      <span class="chevbtn h-chev"></span>
      {#if changedPaths.length}
        <input type="checkbox" class="pick h-pick" checked={allState === "on"} indeterminate={allState === "some"}
          title={allState === "on" ? "Deselect all changes" : "Select all changes"}
          onchange={() => tick(changedPaths, allState !== "on")} />
      {/if}
      <span class="title h-title">{all ? "All files" : "Changed files"}</span>
      {#if changedCount}
        {@const ticked = changedPaths.filter((p) => !excluded[p])}
        <span class="total h-total">{changedCount.toLocaleString()} change{changedCount === 1 ? "" : "s"} · {formatBytes(tree.changedSize)}</span>
        <button class="ghost revert h-revert" disabled={!ticked.length}
          title={!ticked.length ? "Tick changes to discard them" : ticked.length === changedPaths.length
            ? "Discard all changes…" : `Discard the ${ticked.length} ticked change${ticked.length === 1 ? "" : "s"}…`}
          aria-label="Discard the ticked changes"
          onclick={() => (ticked.length === changedPaths.length ? ondiscardall() : ondiscardsome(ticked))}><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M3 12a9 9 0 1 0 9-9 9.75 9.75 0 0 0-6.74 2.74L3 8"/><path d="M3 3v5h5"/></svg></button>
      {/if}
      <label class="all h-all" title="List every file in the project folder">
        All files <input type="checkbox" class="switch" role="switch" bind:checked={all} onchange={rememberAll} />
      </label>
    </div>
    {#if files.length === 0}
      <p class="muted empty">{all ? "The project folder is empty." : (st.tool === "Ableton Live" ? "No uncommitted changes. Work in Live and press Ctrl+S — your changes show up here." : `No uncommitted changes. Work in ${st.tool || "your app"} and save — your changes show up here.`)}</p>
    {:else}
      <ul bind:this={list} style:padding-top="{win.from * ROW}px" style:padding-bottom="{(rows.length - win.to) * ROW}px">
        {#each rows.slice(win.from, win.to) as row (row.file ? row.file.path : "dir:" + row.folder!.path)}
          {#if row.folder}
            {@const d = row.folder}
            <li>
              <span class="indent" style:width="{row.depth * 14}px"></span>
              <button class="ghost chevbtn" onclick={() => toggleFolder(d.path)} aria-label={isOpen(d.path) ? "Close folder" : "Open folder"}>
                <svg class="chev" class:open={isOpen(d.path)} viewBox="0 0 10 10" aria-hidden="true">
                  <path d="M3 1.5 L7 5 L3 8.5" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" />
                </svg>
              </button>
              {#if changedPaths.length}
                {#if d.changed}
                  {@const fs = folderState(d.path)}
                  <input type="checkbox" class="pick" checked={fs === "on"} indeterminate={fs === "some"}
                    title={fs === "some" ? "Some changes in this folder are ticked" : "Commit the changes in this folder"}
                    onchange={() => tick(inside(d.path), fs !== "on")} />
                {:else}<span class="pick"></span>{/if}
              {/if}
              <button class="file dir" class:untracked={!d.tracked} class:changed={d.changed > 0}
                onclick={() => toggleFolder(d.path)} oncontextmenu={(e) => openMenu(e, d.path, true)} title={d.path}>
                <FileIcon kind="folder" open={isOpen(d.path)} faint={!d.tracked} />
                <span class="fname">{d.name}</span>
                {#if d.changed}
                  {@const ff = moves.movedFrom(d.path)}
                  {#if ff}<span class="from" title={`Moved from ${ff}/`}>← {ff}/</span>{/if}
                {/if}
                {#if d.changed && !isOpen(d.path)}<span class="right"><span class="count"
                  title={`${d.changed} changed file${d.changed === 1 ? "" : "s"} inside, ${formatBytes(d.changedSize)}`}>{d.changed} · {formatBytes(d.changedSize)}</span></span>{/if}
              </button>
              <button class="ghost more" title="More" onclick={(e) => { e.stopPropagation(); openMenu(e, d.path, true); }}>⋯</button>
            </li>
          {:else}
            {@const f = row.file!}
            <li>
              <span class="indent" style:width="{row.depth * 14}px"></span>
              <span class="chevbtn"></span>
              {#if changedPaths.length}
                {#if isChange(f)}
                  <input type="checkbox" class="pick" checked={!excluded[f.path]} title="Commit this change"
                    onchange={(e) => tick([f.path], (e.currentTarget as HTMLInputElement).checked)} />
                {:else}<span class="pick"></span>{/if}
              {/if}
              <button class="file {f.status}" class:on={f.path === selected}
                onclick={() => select(f.path)} oncontextmenu={(e) => openMenu(e, f.path)} title={f.path}>
                <FileIcon kind={f.kind} faint={f.status === "ignored" || f.status === "deleted"} />
                <span class="fname">{name(f.path)}</span>
                {#if f.status === "renamed" && !moves.covered(f.path)}
                  <span class="from" title={`Moved from ${f.from}${f.edited ? ", and changed" : ""}`}>← {fromLabel(f)}</span>
                {/if}
                <span class="right">
                  {#if f.live}
                    {@const v = liveShort(f.live)}
                    <span class="live" class:odd={usualLive && v !== usualLive}
                      title={`Saved with ${f.live}${usualLive && v !== usualLive ? ` — most sets here use Live ${usualLive}` : ""}`}>{v}</span>
                  {/if}
                  {#if sym[f.status]}<span class="sym" title={statusName[f.status]}>{sym[f.status]}</span>{/if}
                </span>
              </button>
              <button class="ghost more" title="More" onclick={(e) => { e.stopPropagation(); openMenu(e, f.path); }}>⋯</button>
            </li>
          {/if}
        {/each}
      </ul>
    {/if}
  </aside>
  {#if commitBox}<div class="commit">{@render commitBox()}</div>{/if}
  </div>

  <section class="detail">
    {#if !selected || !current}
      {@render summary()}
    {:else}
      {@const View = viewerFor(current).component}
      <div class="detail-h">
        <div class="title">
          <div class="dname"><FileIcon kind={current.kind} /> {name(selected)}</div>
          <div class="faint small mono">{selected}</div>
          {#if current.live}<div class="faint small">Saved with {current.live}</div>{/if}
        </div>
        {#if current.status !== "ignored"}
          <div class="modes" title="The file as it is, what you changed since the version you're on, or its committed versions">
            <button class:on={fileView.mode === "preview"} onclick={() => setFileMode("preview")}>Preview</button>
            <button class:on={fileView.mode === "changes"} onclick={() => setFileMode("changes")}>Changes</button>
            <button class:on={fileView.mode === "history"} onclick={() => setFileMode("history")}>History</button>
          </div>
        {/if}
      </div>

      {#if current.status === "ignored"}
        <p class="muted">DAWGit doesn't keep this file in versions: the project's rules leave it out (see the project's
          settings, ⚙ at the top).</p>
      {:else if fileView.mode !== "history"}
        {@const v = sides(fileView.mode, current)}
        {#if fileView.mode === "preview"}
          {#if current.status === "deleted"}
            <p class="muted">Deleted since the version you're on: this is how it was.</p>
          {:else if current.size}
            <p class="muted">{formatBytes(current.size)}</p>
          {/if}
        {:else if current.status === "unchanged"}
          <p class="muted">No changes since the version you're on. {formatBytes(current.size)}</p>
        {:else}
          <p class="muted">{current.status === "renamed" ? `Moved from ${current.from}${current.edited ? ", and changed," : ""}`
            : current.status === "added" ? "New file" : current.status === "deleted" ? "Deleted" : "Changed"}
            since the version you're on{current.size ? ` · ${formatBytes(current.size)}` : ""}.</p>
        {/if}
        {#if v.a || v.b}
          <View {root} file={current} a={v.a} b={v.b} compare={v.compare} stamp={loadedAt} />
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
                  {#if h.status === "renamed"}<span class="from" title={`Moved here from ${h.from}`}>← {h.from}</span>{/if}
                  <span class="faint">{h.version.author} · {ago(h.version.time)}</span>
                </button>
              </li>
            {/each}
          </ul>
          {#if picked && history.find((x) => x.version.id === picked)}
            {@const h = history.find((x) => x.version.id === picked)!}
            {@const prev = before(picked)}
            <!-- where the file was in each version (it may have moved since) -->
            {@const hp = h.path || selected}
            {@const pp = prev?.path || selected}
            <div class="picked">
              {#if h.status !== "deleted"}
                <div class="restore">
                  <button onclick={() => onrestore(selected, h.version.id, h.version.message || h.version.short, hp)}
                    title="Put this file back as it was in this version; the rest of the project stays">Restore this version</button>
                  <span class="faint small">Only this file changes; commit it when you're happy.</span>
                </div>
              {/if}
              <p class="muted">{h.status === "renamed" ? `Moved here from ${h.from}` : h.status === "added" ? "Added"
                : h.status === "deleted" ? "Deleted" : "Changed"} in this version.</p>
              <View {root} file={current} compare={true} stamp={loadedAt}
                a={h.status === "deleted" ? null : { path: hp, version: h.version.id, label: versionLabel(h) }}
                b={prev && prev.status !== "deleted" ? { path: pp, version: prev.version.id, label: `Before: ${versionLabel(prev)}` } : null} />
            </div>
          {/if}
        {/if}
      {/if}
    {/if}
  </section>
</div>

{#if menu}
  {@const m = menu}
  {@const f = m.dir ? undefined : files.find((x) => x.path === m.path)}
  <div class="ctx" role="menu" style:left="{Math.min(m.x, window.innerWidth - 240)}px" style:top="{Math.min(m.y, window.innerHeight - 260)}px">
    {#if !m.dir && f && f.status !== "deleted"}
      <button class="item" onclick={() => openFile(m.path)}>{f.kind === "set" ? "Open in Live" : f.kind === "audio" ? "Open in default player" : "Open"}</button>
    {/if}
    {#if m.dir || (f && f.status !== "deleted")}
      <button class="item" onclick={() => { const p = m.path; menu = null; api.ShowFile(root, p).catch((e) => toast(errorText(e), "error")); }}>Show in Explorer</button>
    {/if}
    {#if !m.dir}
      <button class="item" disabled={f?.status === "ignored"} onclick={() => showHistory(m.path)}>View file history</button>
      {#if f?.kind === "audio" && f.status !== "deleted"}
        <button class="item" onclick={() => { converting = m.path; menu = null; }}>Convert…</button>
      {/if}
      {#if canDiscard(f)}
        <button class="item danger-text" onclick={() => { const p = m.path; menu = null; ondiscard(p); }}>Discard changes…</button>
      {/if}
    {/if}
    {#if m.ignore.length}
      <div class="sep"></div>
      <div class="sub" role="none" onmouseenter={() => (ignoreOpen = true)} onmouseleave={() => (ignoreOpen = false)}>
        <button class="item has-sub" onclick={() => (ignoreOpen = !ignoreOpen)} aria-expanded={ignoreOpen}>
          Ignore<span class="arrow">›</span>
        </button>
        {#if ignoreOpen}
          <div class="ctx submenu" role="menu" class:left={m.x > window.innerWidth - 480}>
            {#each m.ignore as o}
              <button class="item" onclick={() => ignore(o.pattern)}>{o.label}<span class="faint pat mono">{o.pattern}</span></button>
            {/each}
            <p class="faint note">Adds a rule to .dawgit.yaml: the files stay on disk, out of versions.</p>
          </div>
        {/if}
      </div>
    {/if}
  </div>
{/if}

{#if converting}
  <ConvertDialog {root} file={converting} onclose={() => (converting = "")}
    ondone={(p) => { converting = ""; toast(`Converted to ${p.slice(p.lastIndexOf("/") + 1)}`, "ok"); reveal(p); }} />
{/if}

<style>
  .panel { display: grid; grid-template-columns: minmax(240px, 34%) 1fr; height: 100%; min-height: 0; }
  .side { display: flex; flex-direction: column; min-height: 0; border-right: 1px solid var(--line); }
  .files { flex: 1; overflow: auto; min-height: 0; padding: 0 8px 16px 0; }
  /* the header stays at the top, set apart from the tree */
  /* the header stays at the top, set apart from the tree; its columns are the rows' (box, then icon) */
  .files { container-type: inline-size; }
  /* under the files: the message and the button, apart from the list */
  .commit { flex: none; border-top: 1px solid var(--line); background: var(--panel); padding: 12px 14px 14px; }
  .commit :global(textarea) { width: 100%; resize: vertical; min-height: 54px; }
  .head { position: sticky; top: 0; z-index: 3; margin: 0 -8px 6px 0; padding: 8px 8px 6px 0;
    background: var(--panel); border-bottom: 1px solid var(--line); font-size: 12px; color: var(--muted);
    display: grid; align-items: center; row-gap: 3px;
    grid-template-columns: 22px 20px minmax(0, 1fr) auto auto;
    grid-template-areas: "chev pick title title all" ". . total discard discard"; }
  @container (min-width: 380px) {
    .head { grid-template-columns: 22px 20px auto minmax(0, 1fr) auto auto;
      grid-template-areas: "chev pick title total discard all"; }
    .h-total { padding-left: 10px; }
    .h-revert { margin-right: 10px; }
  }
  .h-chev { grid-area: chev; margin-left: 4px; }
  .h-pick { grid-area: pick; }
  .h-title { grid-area: title; padding-left: 4px; text-transform: uppercase; letter-spacing: .06em;
    white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .h-total { grid-area: total; padding-left: 4px; color: var(--faint); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .h-revert { grid-area: discard; justify-self: end; } /* (not "revert": a CSS keyword) */
  .h-all { grid-area: all; justify-self: end; }
  .revert { flex: none; display: inline-flex; padding: 3px; border-radius: 5px; color: var(--muted); }
  .revert svg { width: 14px; height: 14px; }
  .revert:hover:not(:disabled) { color: var(--danger); background: #33363d; }
  .revert:disabled { opacity: .35; }
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
  li { position: relative; display: flex; align-items: center; height: 30px; }
  .indent { flex: none; }
  .chevbtn { flex: none; width: 18px; height: 22px; padding: 0; margin-left: 4px; display: flex; align-items: center;
    justify-content: center; border: none; background: transparent; }
  /* The commit box: ticked, unticked, or some of a folder (gray with a dash). */
  /* (padding 0: inputs have padding everywhere, which made the box wide) */
  .pick { appearance: none; position: relative; flex: none; width: 14px; min-width: 14px; height: 14px; padding: 0;
    margin: 0 4px 0 2px; border: 1.5px solid var(--muted); border-radius: 3px; background: transparent; cursor: pointer; }
  .pick:checked { background: var(--accent); border-color: var(--accent); }
  .pick:checked::after { content: ""; position: absolute; left: 3.5px; top: 0.5px; width: 3.5px; height: 7.5px;
    border: solid var(--accent-ink); border-width: 0 2px 2px 0; transform: rotate(45deg); }
  .pick:indeterminate { background: #5b606b; border-color: #5b606b; }
  .pick:indeterminate::after { content: ""; position: absolute; left: 2px; right: 2px; top: 4.5px; height: 2px;
    border-radius: 1px; background: #fff; }
  .pick:focus-visible { outline: 2px solid var(--accent); outline-offset: 1px; }
  span.pick { border-color: transparent; cursor: default; }
  .chev { width: 12px; height: 12px; flex: none; color: var(--muted); transition: transform .12s; }
  .chev.open { transform: rotate(90deg); }
  /* Folders read like files: bright when they hold changes, muted otherwise,
     faint when nothing inside is tracked. */
  .dir .fname { color: var(--muted); }
  .dir.changed .fname { color: var(--text); }
  .dir.untracked .fname { color: var(--faint); }
  .count { font-size: 11px; padding: 0 6px; border-radius: 8px; background: #33363d; color: var(--mod); }
  .file { flex: 1; min-width: 0; display: flex; align-items: center; gap: 6px; border: none; background: transparent;
    padding: 5px 30px 5px 4px; border-radius: 6px; text-align: left; font-size: 13.5px; }
  .file:hover { background: var(--panel); }
  .file.on { background: var(--panel-2); }
  /* What changed, at the end of the row: a small colored square. */
  .right { margin-left: auto; display: flex; align-items: center; gap: 6px; flex: none; }
  .sym { width: 16px; height: 16px; border-radius: 4px; display: inline-flex; align-items: center; justify-content: center;
    font-size: 12px; font-weight: 700; line-height: 1; }
  .file.added .sym { background: rgba(111, 207, 127, .16); }
  .file.deleted .sym { background: rgba(229, 103, 95, .16); }
  .file.modified .sym { background: rgba(106, 176, 243, .16); }
  .file.added .sym { color: var(--add); }
  .file.deleted .sym { color: var(--del); }
  .file.modified .sym { color: var(--mod); }
  .file.deleted .fname { text-decoration: line-through; color: var(--muted); }
  .file.untracked .sym, .file.untracked .fname { color: var(--muted); }
  .file.ignored .fname, .file.unchanged .fname { color: var(--muted); }
  .file.ignored .fname { color: var(--faint); }
  .fname { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .from { flex: 0 1000 auto; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    font-size: 11.5px; color: var(--faint); }
  .file.renamed .sym { color: var(--warn); background: rgba(232, 176, 75, .16); }
  .vsym.renamed { color: var(--warn); }
  .live { font-size: 10.5px; padding: 0 5px; border-radius: 7px; background: #33363d; color: var(--muted);
    font-variant-numeric: tabular-nums; flex: none; }
  .live.odd { background: var(--warn-bg); color: var(--warn); }
  .more { position: absolute; right: 4px; top: 50%; transform: translateY(-50%); visibility: hidden; padding: 0 6px; }
  li:hover .more { visibility: visible; }

  .detail { overflow: auto; min-height: 0; padding: 12px 4px 16px 20px; }
  .detail-h { display: flex; flex-wrap: wrap; align-items: flex-start; gap: 8px 12px; margin-bottom: 10px; }
  .title { flex: 1 1 180px; min-width: 0; }
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

  .ctx { position: fixed; z-index: 40; min-width: 210px; padding: 6px; background: var(--panel-2);
    border: 1px solid var(--line); border-radius: 8px; box-shadow: 0 12px 30px rgba(0, 0, 0, .45); }
  .ctx .item { display: block; width: 100%; border: none; background: transparent; padding: 6px 8px; text-align: left; }
  .ctx .item:hover:not(:disabled) { background: #33363d; }
  .sub { position: relative; }
  .has-sub { display: flex !important; align-items: center; }
  .arrow { margin-left: auto; color: var(--muted); }
  .submenu { position: absolute; left: calc(100% + 2px); top: -6px; min-width: 250px; }
  .submenu.left { left: auto; right: calc(100% + 2px); }
  .submenu .item { display: flex; flex-direction: column; gap: 1px; }
  .pat { font-size: 11px; }
  .note { margin: 6px 8px 2px; font-size: 11px; line-height: 1.4; }
  .danger-text { color: var(--danger); }
  .sep { height: 1px; background: var(--line); margin: 6px 0; }
</style>
