<script lang="ts">
  import { onMount } from "svelte";
  import { Events } from "@wailsio/runtime";
  import { api, errorText, progressShort, progressText, type Overview, type Progress, type TeamProject } from "./lib/api";
  import { toast } from "./lib/notify.svelte";
  import ProjectView from "./lib/ProjectView.svelte";
  import Onboarding from "./lib/Onboarding.svelte";
  import TeamMenu from "./lib/TeamMenu.svelte";
  import Modal from "./lib/Modal.svelte";
  import Toasts from "./lib/Toasts.svelte";

  let overview = $state<Overview | null>(null);
  let onboarding = $state(false);
  // Selected sidebar entry: a project folder, or a team project not here yet.
  let selected = $state<{ root?: string; id?: string }>({});
  let refreshKey = $state(0);
  let busy = $state("");
  let autostart = $state(false);
  let confirmShare = $state<string | null>(null);
  let rowMenu = $state(""); // key of the project whose ⋯ menu is open
  let confirmDelete = $state<TeamProject | null>(null);
  let deleteWord = $state("");
  const rowKey = (p: TeamProject) => p.root || p.id;

  const SELECTED_KEY = "dawgit.selected";
  const DOWNLOAD_DIR_KEY = "dawgit.downloadDir";
  const remember = (k: string, v: string) => { try { localStorage.setItem(k, v); } catch { /* not persisted */ } };
  const recall = (k: string) => { try { return localStorage.getItem(k) ?? ""; } catch { return ""; } };

  let reloading = $state(false);
  let lastReload = 0;
  async function reload() {
    reloading = true;
    lastReload = Date.now();
    try {
      overview = await api.Overview();
    } catch (e) {
      toast(errorText(e), "error");
    } finally {
      reloading = false;
    }
  }

  // New projects on the team show up without a manual refresh: every minute,
  // and when the window comes back to the front.
  function reloadIfStale() {
    if (!onboarding && !reloading && Date.now() - lastReload > 5000) reload();
  }
  $effect(() => {
    const t = setInterval(reloadIfStale, 60000);
    return () => clearInterval(t);
  });

  // Save/upload/download progress per project folder, for the sidebar.
  let activity = $state<Record<string, Progress>>({});
  let lastProgress = $state<Progress | null>(null);
  const activityTimers: Record<string, ReturnType<typeof setTimeout>> = {};
  function onProgress(p: Progress) {
    clearTimeout(activityTimers[p.root]);
    if (p.stage === "done") {
      delete activity[p.root];
      return;
    }
    activity[p.root] = p;
    lastProgress = p;
    // In case the final event is missed.
    activityTimers[p.root] = setTimeout(() => delete activity[p.root], 10 * 60000);
  }

  // A project just added to a team: its view commits and uploads the first
  // version.
  let firstShare = $state("");

  let current = $derived(overview?.teams.find((t) => t.id === overview?.currentTeam));
  let entries = $derived(overview?.projects ?? []);
  let selectedEntry = $derived.by(() => {
    const all = [...entries, ...(overview?.local ?? [])];
    return all.find((p) => (selected.root ? p.root === selected.root : !!selected.id && p.id === selected.id && !p.root));
  });

  function select(p: TeamProject) {
    selected = p.root ? { root: p.root } : { id: p.id };
    if (p.root) remember(SELECTED_KEY, p.root);
  }

  // Keep a valid selection when the team or the list changes.
  $effect(() => {
    if (!overview || selectedEntry) return;
    const last = recall(SELECTED_KEY);
    const all = [...entries, ...overview.local];
    const pick = all.find((p) => p.root && p.root === last) ?? all.find((p) => p.status === "downloaded") ?? all[0];
    if (pick) select(pick);
  });

  async function download(p: TeamProject) {
    let parent = recall(DOWNLOAD_DIR_KEY);
    if (!parent) {
      parent = await api.ChooseFolder("Where should downloaded projects go?");
      if (!parent) return;
      remember(DOWNLOAD_DIR_KEY, parent);
    }
    busy = p.id;
    lastProgress = null;
    try {
      const got = await api.DownloadProject(overview!.currentTeam, p.id, parent);
      toast(`Downloaded “${got.name}” to ${got.root}`, "ok", 7000);
      await reload();
      select(got);
    } catch (e) {
      toast(errorText(e), "error", 9000);
    } finally {
      busy = "";
    }
  }

  async function changeDownloadDir() {
    const d = await api.ChooseFolder("Where should downloaded projects go?");
    if (d) remember(DOWNLOAD_DIR_KEY, d);
  }

  async function locate(p: TeamProject) {
    const folder = await api.ChooseFolder(`Where is “${p.name}” now?`);
    if (!folder) return;
    try {
      const got = await api.LocateProject(overview!.currentTeam, p.id, folder);
      await reload();
      select(got);
    } catch (e) {
      toast(errorText(e), "error");
    }
  }

  async function forget(p: TeamProject) {
    await api.ForgetProject(p.root);
    toast(`Removed “${p.name}” from this computer's list (files untouched)`, "info");
    selected = {};
    await reload();
  }

  async function deleteFromTeam(p: TeamProject) {
    confirmDelete = null;
    busy = "delete";
    try {
      await api.DeleteProjectFromTeam(overview!.currentTeam, p.id);
      toast(p.root ? `Deleted “${p.name}” from ${current?.name}. Your copy is kept on this computer.`
        : `Deleted “${p.name}” from ${current?.name}`, "info", 8000);
      if (!p.root) selected = {};
      await reload();
    } catch (e) {
      toast(errorText(e), "error", 9000);
    } finally {
      busy = "";
    }
  }

  async function addToTeam() {
    const folder = await api.ChooseFolder(`Choose an Ableton project folder to share with ${current?.name ?? "the team"}`);
    if (folder) confirmShare = folder;
  }

  async function share(folder: string) {
    confirmShare = null;
    busy = "add";
    try {
      const got = await api.AddProjectToTeam(overview!.currentTeam, folder);
      firstShare = got.root;
      await reload();
      select(got);
    } catch (e) {
      toast(errorText(e), "error", 9000);
    } finally {
      busy = "";
    }
  }

  async function addLocal() {
    const folder = await api.ChooseFolder("Choose an Ableton project folder (kept on this computer only)");
    if (!folder) return;
    try {
      const p = await api.AddLocalProject(folder);
      await reload();
      select(p);
    } catch (e) {
      toast(errorText(e), "error");
    }
  }

  async function toggleAutostart(on: boolean) {
    try {
      await api.SetAutostart(on);
      autostart = on;
    } catch (e) {
      toast(errorText(e), "error");
    }
  }

  type AgentEvent = { root: string; kind: string; author: string; labels: string[]; text: string; versions: { author: string; message: string }[] };

  onMount(() => {
    reload().then(() => {
      if (overview && overview.teams.length === 0 && overview.local.length === 0) onboarding = true;
    });
    api.Autostart().then((on) => (autostart = on)).catch(() => {});
    const offProgress = Events.On("progress", (ev: { data: Progress }) => onProgress(ev.data));
    const offAgent = Events.On("agent", (ev: { data: AgentEvent }) => {
      const e = ev.data;
      const all = [...entries, ...(overview?.local ?? [])];
      const name = all.find((p) => p.root === e.root)?.name ?? "";
      switch (e.kind) {
        case "new-versions":
          toast(`${name}: ${e.versions.map((v) => `${v.author} saved “${v.message}”`).join("\n")}`, "info", 8000);
          break;
        case "teammate-editing":
          toast(`${name}: ${e.author} is editing ${e.labels.map((l) => l.split(": ").slice(1).join(": ")).join(", ")}`, "info");
          break;
        case "overlap":
          toast(`${name}: ${e.text}`, "warn", 9000);
          break;
      }
      if (e.root === selected.root && e.kind !== "backed-up") refreshKey++;
    });
    return () => {
      offProgress();
      offAgent();
    };
  });

  const statusText: Record<string, string> = { remote: "not downloaded", missing: "folder not found", local: "not shared" };
  const statusIcon: Record<string, string> = { remote: "☁", missing: "⚠", downloaded: "♪", local: "♪" };
</script>

<svelte:window onfocus={reloadIfStale}
  onclick={(e) => { if (rowMenu && !(e.target as HTMLElement).closest(".row-menu, .more")) rowMenu = ""; }} />

{#if !overview}
  <div class="loading faint">Loading…</div>
{:else if onboarding}
  <Onboarding {overview} {reload} onfinish={async (root, share) => {
    onboarding = false;
    if (root && share) firstShare = root;
    await reload();
    if (root) selected = { root };
  }} />
{:else}
  <div class="shell">
    <aside>
      <div class="brand"><img src="/icon.png" alt="" /> DAWGit</div>
      <TeamMenu {overview} {reload} />

      <div class="list">
        {#if current}
          <div class="section row-h">
            <span>Projects</span>
            <button class="ghost tiny" class:spin={reloading} onclick={reload} title="Check the team for new projects">↻</button>
          </div>
          {#if overview.teamError}
            <div class="offline" title={overview.teamError}>● {current.isStorage ? "Storage" : "Server"} not reachable</div>
          {/if}
          <ul>
            {#each entries as p (p.root || p.id)}
              {@render row(p)}
            {:else}
              <li class="empty faint">No projects in this team yet.</li>
            {/each}
          </ul>
          <button class="add" onclick={addToTeam} disabled={busy === "add"}>+ Add local project</button>
        {:else}
          <p class="faint small pad">Connect to a team to share projects.</p>
        {/if}

        {#if overview.local.length}
          <div class="section">On this computer only</div>
          <ul>
            {#each overview.local as p (p.root)}
              {@render row(p)}
            {/each}
          </ul>
        {/if}
      </div>

      <div class="bottom">
        <button class="link" onclick={addLocal}>Keep a project on this computer only…</button>
        <label class="autostart" title="Keeps DAWGit in the tray so teammates see what you edit and you hear about new versions">
          <input type="checkbox" checked={autostart} onchange={(e) => toggleAutostart(e.currentTarget.checked)} />
          Start with Windows
        </label>
      </div>
    </aside>

    <section class="content">
      {#if selectedEntry && (selectedEntry.status === "downloaded" || selectedEntry.status === "local")}
        {#key selectedEntry.root}
          <ProjectView root={selectedEntry.root} {refreshKey} teams={overview.teams} onchanged={reload}
            firstShare={firstShare === selectedEntry.root} onfirstshared={() => (firstShare = "")} />
        {/key}
      {:else if selectedEntry && selectedEntry.status === "remote"}
        {@const p = selectedEntry}
        <div class="placeholder">
          <div class="big" aria-hidden="true">☁</div>
          <h1>{p.name}</h1>
          <p class="muted">This song is on {current?.name} but not on this computer yet.</p>
          <button class="primary" onclick={() => download(p)} disabled={!!busy}>
            {busy === p.id ? "Downloading…" : "↓ Download"}
          </button>
          {#if busy === p.id && lastProgress}
            <div class="dl-progress">
              <span class="small muted">{progressText(lastProgress)}</span>
              {#if lastProgress.total}
                <div class="bar"><div style="width: {Math.round((100 * lastProgress.done) / lastProgress.total)}%"></div></div>
              {/if}
            </div>
          {/if}
          <p class="faint small">Into {recall(DOWNLOAD_DIR_KEY) || "a folder you choose"} ·
            <button class="link" onclick={changeDownloadDir}>change</button></p>
        </div>
      {:else if selectedEntry && selectedEntry.status === "missing"}
        {@const p = selectedEntry}
        <div class="placeholder">
          <div class="big" aria-hidden="true">⚠</div>
          <h1>{p.name}</h1>
          <p class="muted">The project folder was moved or deleted:<br /><span class="mono">{p.root}</span></p>
          <div class="row center">
            <button class="primary" onclick={() => locate(p)}>Locate folder…</button>
            <button onclick={() => download(p)} disabled={!!busy}>Download again</button>
            <button class="ghost" onclick={() => forget(p)}>Remove from list</button>
          </div>
        </div>
      {:else}
        <div class="placeholder">
          <h1>{current ? current.name : "DAWGit"}</h1>
          <p class="muted">{current ? "Pick a song on the left, or add a local project to share it with the team." : "Connect to a team from the menu at the top left."}</p>
        </div>
      {/if}
    </section>
  </div>
{/if}

{#snippet row(p: TeamProject)}
  <li>
    <button class="proj {p.status}" class:on={selectedEntry === p} onclick={() => select(p)}
      title={p.status === "remote" ? "On the team, not on this computer yet" : p.root}>
      <span class="icon" aria-hidden="true">{statusIcon[p.status]}</span>
      <span class="text">
        <span class="name">{p.name}</span>
        <span class="meta" class:busy={p.root && activity[p.root]}>
          {p.root && activity[p.root] ? progressShort(activity[p.root]) : statusText[p.status] ?? `⑂ ${p.branch}`}
        </span>
      </span>
    </button>
    <button class="ghost more" class:open={rowMenu === rowKey(p)} title="More"
      onclick={() => (rowMenu = rowMenu === rowKey(p) ? "" : rowKey(p))}>⋯</button>
    {#if rowMenu === rowKey(p)}
      <div class="row-menu" role="menu">
        {#if p.root}
          <button class="item" onclick={() => { rowMenu = ""; forget(p); }}>
            Remove from list<span class="faint">the folder stays on this computer</span>
          </button>
        {/if}
        {#if p.status !== "local"}
          <button class="item danger-text" onclick={() => { rowMenu = ""; deleteWord = ""; confirmDelete = p; }}>
            Delete from server…<span class="faint">for everyone in {current?.name}</span>
          </button>
        {/if}
      </div>
    {/if}
  </li>
{/snippet}

{#if confirmShare}
  {@const folder = confirmShare}
  <Modal title="Share with {current?.name}?" onclose={() => (confirmShare = null)}>
    <p class="mono">{folder}</p>
    <p class="muted">DAWGit saves a first version of this project and uploads it, including its samples, so your
      teammates can download it.</p>
    {#snippet footer()}
      <button onclick={() => (confirmShare = null)}>Cancel</button>
      <button class="primary" onclick={() => share(folder)}>Share</button>
    {/snippet}
  </Modal>
{/if}

{#if confirmDelete}
  {@const p = confirmDelete}
  <Modal title="Delete “{p.name}” from the server?" onclose={() => (confirmDelete = null)}>
    <p>This removes the song and all its versions from <strong>{current?.name}</strong>, for everyone in the team.
      Copies already on someone's computer are not touched{p.root ? " — yours stays here as a project on this computer only" : ""}.</p>
    <label for="delete-word">Type <strong>{p.name}</strong> to confirm</label>
    <input id="delete-word" class="confirm-input" bind:value={deleteWord} autocomplete="off"
      onkeydown={(e) => { if (e.key === "Enter" && deleteWord.trim() === p.name) deleteFromTeam(p); }} />
    {#snippet footer()}
      <button onclick={() => (confirmDelete = null)}>Cancel</button>
      <button class="danger" disabled={deleteWord.trim() !== p.name} onclick={() => deleteFromTeam(p)}>Delete</button>
    {/snippet}
  </Modal>
{/if}

<Toasts />

<style>
  .loading { padding: 40px; }
  .shell { display: grid; grid-template-columns: 250px 1fr; height: 100%; }
  aside { background: #141518; border-right: 1px solid var(--line); display: flex; flex-direction: column; padding: 12px 10px; min-height: 0; }
  .brand { display: flex; align-items: center; gap: 8px; font-weight: 700; font-size: 15px; padding: 2px 8px 10px; }
  .brand img { width: 20px; height: 20px; }
  .list { flex: 1; overflow: auto; min-height: 0; }
  .section { font-size: 11px; text-transform: uppercase; letter-spacing: .06em; color: var(--faint); padding: 8px 8px 4px; }
  .row-h { display: flex; align-items: center; justify-content: space-between; }
  .tiny { padding: 0 6px; font-size: 13px; line-height: 18px; color: var(--faint); }
  .tiny:hover { color: var(--text); }
  .spin { animation: spin .8s linear infinite; }
  @keyframes spin { to { transform: rotate(360deg); } }
  .meta.busy { color: var(--accent); }
  .dl-progress { width: 280px; margin: 10px auto 0; display: flex; flex-direction: column; gap: 6px; }
  .bar { height: 4px; border-radius: 2px; background: var(--line); overflow: hidden; }
  .bar > div { height: 100%; background: var(--accent); transition: width .2s; }
  ul { list-style: none; margin: 0; padding: 0; }
  li { display: flex; align-items: center; position: relative; }
  .proj { flex: 1; min-width: 0; display: flex; align-items: center; gap: 10px; border: none; background: transparent; padding: 7px 28px 7px 10px; border-radius: 8px; text-align: left; }
  .proj:hover { background: var(--panel); }
  .proj.on { background: var(--panel-2); }
  .icon { width: 16px; text-align: center; color: var(--accent); }
  .text { display: flex; flex-direction: column; min-width: 0; }
  .name { font-weight: 600; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .meta { font-size: 12px; color: var(--faint); }
  /* Not on this computer: dimmed, with a cloud icon (not colour alone). */
  .proj.remote .name { color: var(--muted); font-weight: 500; }
  .proj.remote .icon { color: var(--faint); }
  .proj.missing .icon, .proj.missing .meta { color: var(--warn); }
  .proj.local .icon { color: var(--mod); }
  .more {
    position: absolute; top: 4px; right: 4px; visibility: hidden; padding: 0 6px; line-height: 18px;
    font-size: 15px; color: var(--muted); border-radius: 6px;
  }
  li:hover .more, .more.open { visibility: visible; }
  .more:hover, .more.open { background: #33363d; color: var(--text); }
  .row-menu {
    position: absolute; top: 26px; right: 4px; z-index: 30; min-width: 220px; padding: 6px;
    background: var(--panel-2); border: 1px solid var(--line); border-radius: 8px; box-shadow: 0 12px 30px rgba(0, 0, 0, .45);
  }
  .row-menu .item {
    display: flex; flex-direction: column; align-items: flex-start; gap: 1px; width: 100%; border: none;
    background: transparent; padding: 6px 8px; text-align: left;
  }
  .row-menu .item:hover { background: #33363d; }
  .row-menu .item .faint { font-size: 11px; }
  .danger-text { color: var(--danger); }
  .confirm-input { width: 100%; margin-top: 6px; }
  .empty { padding: 6px 10px; font-size: 13px; }
  .offline { font-size: 12px; color: var(--danger); padding: 0 8px 4px; }
  .add { width: 100%; margin: 8px 0 4px; }
  .pad { padding: 0 8px; }
  .bottom { padding-top: 10px; border-top: 1px solid var(--line); display: flex; flex-direction: column; gap: 8px; }
  .link { border: none; background: none; color: var(--muted); text-decoration: underline; padding: 0; font-size: 12.5px; text-align: left; }
  .autostart { display: flex; align-items: center; gap: 8px; margin: 0; font-size: 12.5px; cursor: pointer; }
  .autostart input { width: auto; }
  .small { font-size: 12.5px; }
  .content { min-width: 0; overflow: hidden; }
  .placeholder { height: 100%; display: flex; flex-direction: column; align-items: center; justify-content: center; text-align: center; padding: 40px; }
  .placeholder h1 { margin: 6px 0; }
  .big { font-size: 44px; color: var(--faint); }
  .center { justify-content: center; }
</style>
