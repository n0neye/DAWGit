<script lang="ts">
  import { onMount } from "svelte";
  import { Events } from "@wailsio/runtime";
  import { api, errorText, progressShort, type Overview, type Progress, type ProjectInfo, type TeamProject } from "./lib/api";
  import ProgressBar from "./lib/ProgressBar.svelte";
  import { toast } from "./lib/notify.svelte";
  import ProjectView from "./lib/ProjectView.svelte";
  import Onboarding from "./lib/Onboarding.svelte";
  import TeamMenu from "./lib/TeamMenu.svelte";
  import VerifyDialog from "./lib/VerifyDialog.svelte";
  import Modal from "./lib/Modal.svelte";
  import Toasts from "./lib/Toasts.svelte";
  import ProjectSettings from "./lib/ProjectSettings.svelte";

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
  // Moving a project out of its team (to Local) or into another team.
  let confirmLocal = $state<TeamProject | null>(null);
  let checking = $state<TeamProject | null>(null); // Check project…
  let localFull = $state(false); // also download older versions' files
  let localSize = $state(0);
  const mb = (n: number) => (n >= 1 << 30 ? `${(n / (1 << 30)).toFixed(1)} GB` : `${Math.max(1, Math.round(n / (1 << 20)))} MB`);
  let moving = $state<{ p: TeamProject; team: string } | null>(null);
  let deleteWord = $state("");
  const rowKey = (p: TeamProject) => p.root || p.id;
  let settingsFor = $state<TeamProject | null>(null); // Project settings
  // The ⋯ menu's "Open in" (read when the menu opens).
  let menuInfo = $state<ProjectInfo | null>(null);
  let openSub = $state(false);
  // The list of sets beside the menu: placed on the window (the sidebar's
  // list scrolls, and would cut it off), kept open while the pointer
  // crosses over to it.
  let subAt = $state({ left: 0, top: 0 });
  let subTimer: ReturnType<typeof setTimeout> | undefined;
  function showSub(e: MouseEvent) {
    clearTimeout(subTimer);
    const r = (e.currentTarget as HTMLElement).getBoundingClientRect();
    subAt = { left: r.right - 2, top: r.top - 6 };
    openSub = true;
  }
  function hideSub() {
    clearTimeout(subTimer);
    subTimer = setTimeout(() => (openSub = false), 180);
  }
  function toggleMenu(p: TeamProject) {
    const key = rowKey(p);
    rowMenu = rowMenu === key ? "" : key;
    menuInfo = null;
    openSub = false;
    if (rowMenu && p.root && (p.status === "downloaded" || p.status === "local")) {
      api.ProjectInfo(p.root).then((i) => { if (rowMenu === key) menuInfo = i; }).catch(() => {});
    }
  }
  const toolName = (tool: string) => (tool === "Ableton Live" ? "Live" : tool || "its program");
  function openIn(p: TeamProject, rel: string) {
    rowMenu = "";
    api.OpenInTool(p.root, rel).catch((e) => toast(errorText(e), "error"));
  }
  const openLabel = (p: TeamProject, rel: string) => (rel === "." ? p.name : rel);

  const SELECTED_KEY = "dawgit.selected";
  // Pinned projects come first in the list (this computer only).
  const PINNED_KEY = "dawgit.pinned";
  let pinned = $state<string[]>((() => { try { return JSON.parse(localStorage.getItem(PINNED_KEY) ?? "[]"); } catch { return []; } })());
  function togglePin(p: TeamProject) {
    const key = rowKey(p);
    pinned = pinned.includes(key) ? pinned.filter((k) => k !== key) : [...pinned, key];
    try { localStorage.setItem(PINNED_KEY, JSON.stringify(pinned)); } catch { /* not remembered */ }
  }
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
  let appVersion = $state("");
  let edition = $state(""); // a build with extensions, e.g. "Pro"

  // A newer release on GitHub: offered until the user hides that version.
  let update = $state<{ version: string; pageUrl: string; downloadUrl: string } | null>(null);
  const DISMISSED_KEY = "dawgit.dismissedUpdate";
  async function checkUpdate() {
    try {
      const u = await api.CheckUpdate();
      update = u && u.version !== recall(DISMISSED_KEY) ? u : null;
    } catch {
      /* offline: try again later */
    }
  }
  $effect(() => {
    const t = setInterval(checkUpdate, 6 * 3600 * 1000);
    return () => clearInterval(t);
  });
  function openLink(url: string) {
    api.OpenURL(url).catch(() => window.open(url, "_blank"));
  }

  let current = $derived(overview?.teams.find((t) => t.id === overview?.currentTeam));
  // Local (this computer only) is picked in the team menu like a team.
  let isLocal = $derived(!current);
  let entries = $derived.by(() => {
    const list = (isLocal ? overview?.local : overview?.projects) ?? [];
    const pin = (p: TeamProject) => (pinned.includes(rowKey(p)) ? 0 : 1);
    return [...list].sort((a, b) => pin(a) - pin(b)); // stable: pinned first, the rest as they come
  });
  let selectedEntry = $derived(
    entries.find((p) => (selected.root ? p.root === selected.root : !!selected.id && p.id === selected.id && !p.root)));

  function select(p: TeamProject) {
    selected = p.root ? { root: p.root } : { id: p.id };
    if (p.root) remember(SELECTED_KEY, p.root);
  }

  // Keep a valid selection when the team or the list changes.
  $effect(() => {
    if (!overview || selectedEntry) return;
    const last = recall(SELECTED_KEY);
    const pick = entries.find((p) => p.root && p.root === last) ??
      entries.find((p) => p.status === "downloaded" || p.status === "local") ?? entries[0];
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
    toast(`Unlinked “${p.name}”: the folder and its versions are untouched`, "info");
    selected = {};
    await reload();
  }

  async function deleteFromTeam(p: TeamProject) {
    confirmDelete = null;
    busy = "delete";
    try {
      await api.DeleteProjectFromTeam(overview!.currentTeam, p.id);
      toast(p.root ? `Deleted “${p.name}” from ${current?.name}. Your copy is kept under Local.`
        : `Deleted “${p.name}” from ${current?.name}`, "info", 8000);
      if (!p.root) selected = {};
      await reload();
    } catch (e) {
      toast(errorText(e), "error", 9000);
    } finally {
      busy = "";
    }
  }

  async function moveToLocal(p: TeamProject) {
    confirmLocal = null;
    busy = "move";
    try {
      await api.MoveProjectToLocal(p.root, localFull);
      toast(`“${p.name}” is under Local now; ${current?.name ?? "the team"} keeps its copy`, "info", 7000);
      selected = {};
      await reload();
    } catch (e) {
      toast(errorText(e), "error", 9000);
    } finally {
      busy = "";
    }
  }

  const canMoveTeam = (p: TeamProject) =>
    !!p.root && (p.status === "local" ? (overview?.teams.length ?? 0) > 0 : (overview?.teams.length ?? 0) > 1);

  // Candidate teams for a move: all but the project's own.
  let moveTargets = $derived(moving
    ? (overview?.teams ?? []).filter((t) => moving!.p.status === "local" || t.id !== overview?.currentTeam) : []);

  async function moveToTeam() {
    if (!moving) return;
    const { p, team } = moving;
    moving = null;
    busy = "add";
    try {
      const got = await api.MoveProjectToTeam(p.root, team);
      firstShare = got.root; // share its versions with the new team
      await reload();
      select(got);
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
    api.Version().then((v) => (appVersion = v)).catch(() => {});
    api.Edition().then((e) => (edition = e)).catch(() => {});
    checkUpdate();
    const offProgress = Events.On("progress", (ev: { data: Progress }) => onProgress(ev.data));
    const offAgent = Events.On("agent", (ev: { data: AgentEvent }) => {
      const e = ev.data;
      const all = [...entries, ...(overview?.local ?? [])];
      const name = all.find((p) => p.root === e.root)?.name ?? "";
      switch (e.kind) {
        case "new-versions":
          toast(`${name}: ${e.versions.map((v) => `${v.author} saved “${v.message}”`).join("\n")}`, "info", 8000);
          break;
      }
      if (e.root === selected.root) refreshKey++;
    });
    return () => {
      offProgress();
      offAgent();
    };
  });

  const statusText: Record<string, string> = { remote: "not downloaded", missing: "folder not found" };
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
      <div class="brand">
        <img src="/icon.png" alt="" /> DAWGit
        {#if edition}<span class="edition" title="A DAWGit build with extensions">{edition}</span>{/if}
        {#if appVersion}<span class="version faint" title="DAWGit version">v{appVersion}</span>{/if}
      </div>
      {#if update}
        {@const u = update}
        <div class="update">
          <div class="update-h">
            <span>DAWGit{edition ? ` ${edition}` : ""} {u.version} is available</span>
            <button class="ghost x" title="Hide until the next version"
              onclick={() => { remember(DISMISSED_KEY, u.version); update = null; }}>✕</button>
          </div>
          <div class="update-a">
            {#if u.downloadUrl}
              <button class="primary" onclick={() => openLink(u.downloadUrl)}
                title="Download the installer; run it to update (your projects and teams are kept)">Download</button>
            {/if}
            <button class="ghost" onclick={() => openLink(u.pageUrl)}>What's new</button>
          </div>
        </div>
      {/if}
      <TeamMenu {overview} {reload} />
      {#if current?.keysUnreadable}
        <p class="keys-warn">This computer can't read the keys of “{current.name}” (DAWGit's settings came from
          another computer or Windows user). Enter them again in the team's settings (⚙).</p>
      {/if}

      <div class="list">
        <div class="section row-h">
          <span>Projects</span>
          {#if current}
            <button class="ghost tiny" class:spin={reloading} onclick={reload} title="Check the team for new projects">↻</button>
          {/if}
        </div>
        {#if current && overview.teamError}
          <div class="offline" title={overview.teamError}>● {current.isStorage ? "Storage" : "Server"} not reachable</div>
        {/if}
        <ul>
          {#each entries as p (p.root || p.id)}
            {@render row(p)}
          {:else}
            <li class="empty faint">{current ? "No projects in this team yet." : "No projects on this computer yet."}</li>
          {/each}
        </ul>
        <button class="add" onclick={current ? addToTeam : addLocal} disabled={busy === "add"}>
          <span>+ Add project</span>
          <span class="hint">Select project folder</span>
        </button>
      </div>

      <div class="bottom">
        <label class="autostart" title="Keeps DAWGit in the tray so you hear about new versions from your team">
          <input type="checkbox" checked={autostart} onchange={(e) => toggleAutostart(e.currentTarget.checked)} />
          Start with Windows
        </label>
      </div>
    </aside>

    <section class="content">
      {#if selectedEntry && (selectedEntry.status === "downloaded" || selectedEntry.status === "local")}
        {#key selectedEntry.root}
          <ProjectView root={selectedEntry.root} {refreshKey} teams={overview.teams} onchanged={reload}
            onsettings={() => (settingsFor = selectedEntry ?? null)}
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
            <div class="dl-progress"><ProgressBar p={lastProgress} /></div>
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
            <button class="ghost" onclick={() => forget(p)} title="DAWGit stops listing it; nothing is deleted">Unlink folder</button>
          </div>
        </div>
      {:else}
        <div class="placeholder">
          <h1>{current ? current.name : "Local"}</h1>
          <p class="muted">{current ? "Pick a song on the left, or add a project to share it with the team."
            : "Projects here keep their versions on this computer only. Pick one on the left, or add one."}</p>
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
        <span class="name">{p.name}{#if pinned.includes(rowKey(p))}<svg class="pin" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-label="Pinned"><title>Pinned</title><path d="M12 17v5"/><path d="M9 10.76a2 2 0 0 1-1.11 1.79l-1.78.9A2 2 0 0 0 5 15.24V16a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1v-.76a2 2 0 0 0-1.11-1.79l-1.78-.9A2 2 0 0 1 15 10.76V7a1 1 0 0 1 1-1 2 2 0 0 0 0-4H8a2 2 0 0 0 0 4 1 1 0 0 1 1 1z"/></svg>{/if}</span>
        <span class="meta" class:busy={p.root && activity[p.root]}>
          {p.root && activity[p.root] ? progressShort(activity[p.root]) : statusText[p.status] ?? `⑂ ${p.branch}`}
        </span>
      </span>
    </button>
    <button class="ghost more" class:open={rowMenu === rowKey(p)} title="More"
      onclick={() => toggleMenu(p)}>⋯</button>
    {#if rowMenu === rowKey(p)}
      <div class="row-menu" role="menu">
        <button class="item" onclick={() => { rowMenu = ""; togglePin(p); }}>{pinned.includes(rowKey(p)) ? "Unpin" : "Pin to top"}</button>
        {#if menuInfo && menuInfo.openable.length === 1}
          <button class="item" onclick={() => openIn(p, menuInfo!.openable[0])}>Open in {toolName(menuInfo.tool)}</button>
        {:else if menuInfo && menuInfo.openable.length > 1}
          <div class="sub" role="none" onmouseenter={showSub} onmouseleave={hideSub}>
            <button class="item has-sub" aria-expanded={openSub}>
              Open in {toolName(menuInfo.tool)}<span class="arrow">›</span>
            </button>
            {#if openSub}
              <div class="row-menu submenu" role="menu" tabindex="-1" style:left="{subAt.left}px" style:top="{subAt.top}px"
                onmouseenter={() => clearTimeout(subTimer)} onmouseleave={hideSub}>
                {#each menuInfo.openable as rel}
                  <button class="item" onclick={() => openIn(p, rel)}>{openLabel(p, rel)}</button>
                {/each}
              </div>
            {/if}
          </div>
        {/if}
        {#if p.root && p.status !== "missing"}
          <button class="item" onclick={() => { rowMenu = ""; api.ShowFolder(p.root); }}>Open folder</button>
        {/if}
        <div class="sep"></div>
        <button class="item" onclick={() => { rowMenu = ""; settingsFor = p; }}>Settings…</button>
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

{#if settingsFor}
  {@const p = settingsFor}
  <ProjectSettings {p} team={p.status === "local" ? undefined : current} canMoveTeam={canMoveTeam(p)}
    onclose={() => (settingsFor = null)}
    onrenamed={async () => { await reload(); refreshKey++; settingsFor = entries.find((e) => rowKey(e) === rowKey(p)) ?? null; }}
    oncheck={() => { settingsFor = null; checking = p; }}
    onmovelocal={() => {
      settingsFor = null; localFull = false; localSize = 0; confirmLocal = p;
      api.HistoryDownloadSize(p.root, "").then((n) => (localSize = n)).catch(() => {});
    }}
    onmoveteam={() => { settingsFor = null; moving = { p, team: "" }; }}
    ondelete={() => { settingsFor = null; deleteWord = ""; confirmDelete = p; }}
    onunlink={() => { settingsFor = null; forget(p); }}
    onlocate={() => { settingsFor = null; locate(p); }} />
{/if}

{#if checking}
  <VerifyDialog root={checking.root} name={checking.name} onclose={() => (checking = null)} />
{/if}

{#if confirmLocal}
  {@const p = confirmLocal}
  <Modal title="Move “{p.name}” to Local?" onclose={() => (confirmLocal = null)}>
    <p>The project leaves <strong>{current?.name}</strong> on this computer only: you keep its versions and can go
      on committing here. The team keeps its copy, and your teammates are not affected.</p>
    <p class="muted">To share it again, join the team (or move it to a team) later.</p>
    {#if localSize > 0}
      <label class="full">
        <input type="checkbox" bind:checked={localFull} />
        <span>Also download the files of older versions ({mb(localSize)})
          <span class="faint">Without them, older versions that use other samples than today's need the team again
            to open.</span></span>
      </label>
    {/if}
    {#snippet footer()}
      <button onclick={() => (confirmLocal = null)}>Cancel</button>
      <button class="primary" onclick={() => moveToLocal(p)}>Move to Local</button>
    {/snippet}
  </Modal>
{/if}

{#if moving}
  {@const m = moving}
  <Modal title="Move “{m.p.name}” to a team" onclose={() => (moving = null)}>
    <p class="muted">DAWGit shares the project's versions with the team you pick{m.p.status === "local" ? "" :
      `; ${current?.name} keeps its copy, but you'll no longer get its changes here`}.</p>
    <div class="teams-pick">
      {#each moveTargets as t (t.id)}
        <label><input type="radio" bind:group={m.team} value={t.id} /> {t.name}</label>
      {/each}
    </div>
    {#snippet footer()}
      <button onclick={() => (moving = null)}>Cancel</button>
      <button class="primary" disabled={!m.team} onclick={moveToTeam}>Move</button>
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
  .dl-progress { width: 360px; max-width: 100%; margin: 10px auto 0; display: flex; text-align: left; }
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
  .row-menu .sep { height: 1px; background: var(--line); margin: 4px 2px; }
  .row-menu .sub { position: relative; }
  .row-menu .has-sub { flex-direction: row; justify-content: space-between; align-items: center; }
  .row-menu .arrow { color: var(--faint); }
  .row-menu.submenu { position: fixed; right: auto; min-width: 200px; max-width: 340px; z-index: 40; }
  .row-menu.submenu .item { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; display: block; }
  .row-menu .has-sub[aria-expanded="true"] { background: #33363d; }
  .pin { width: 11px; height: 11px; margin-left: 5px; color: var(--faint); vertical-align: -1px; flex: none; }
  .full { display: flex; gap: 10px; align-items: flex-start; margin: 12px 0 0; color: var(--text); font-size: 14px; }
  .full input { width: auto; margin-top: 3px; }
  .full .faint { display: block; font-size: 12px; margin-top: 2px; }
  .teams-pick { display: flex; flex-direction: column; gap: 8px; margin-top: 12px; }
  .teams-pick label { display: flex; align-items: center; gap: 8px; margin: 0; color: var(--text); font-size: 14px; }
  .teams-pick input { width: auto; }
  .danger-text { color: var(--danger); }
  .confirm-input { width: 100%; margin-top: 6px; }
  .empty { padding: 6px 10px; font-size: 13px; }
  .offline { font-size: 12px; color: var(--danger); padding: 0 8px 4px; }
  .add { width: 100%; margin: 8px 0 4px; display: flex; flex-direction: column; align-items: center; gap: 0; padding: 5px 10px; line-height: 1.3; }
  .add .hint { font-size: 11px; color: var(--faint); font-weight: 400; }
  .pad { padding: 0 8px; }
  .bottom { padding-top: 10px; border-top: 1px solid var(--line); display: flex; flex-direction: column; gap: 8px; }
  .link { border: none; background: none; color: var(--muted); text-decoration: underline; padding: 0; font-size: 12.5px; text-align: left; }
  .version { margin-left: auto; font-size: 11px; font-weight: 400; }
  .edition { font-size: 10px; font-weight: 700; letter-spacing: .06em; text-transform: uppercase; padding: 1px 6px;
    border-radius: 6px; color: var(--accent-ink); background: var(--accent); }
  .update { margin: 0 0 10px; padding: 8px 10px; border-radius: 8px; background: #1f3b35; border: 1px solid #2c5a4e; font-size: 12.5px; }
  .update-h { display: flex; align-items: center; gap: 6px; font-weight: 600; color: var(--accent); }
  .update-h span { flex: 1; }
  .update-a { display: flex; gap: 6px; margin-top: 6px; }
  .update-a button { padding: 3px 10px; font-size: 12.5px; }
  .x { padding: 0 5px; line-height: 16px; color: var(--muted); }
  .autostart { display: flex; align-items: center; gap: 8px; margin: 0; font-size: 12.5px; cursor: pointer; }
  .autostart input { width: auto; }
  .small { font-size: 12.5px; }
  .content { min-width: 0; overflow: hidden; }
  .placeholder { height: 100%; display: flex; flex-direction: column; align-items: center; justify-content: center; text-align: center; padding: 40px; }
  .placeholder h1 { margin: 6px 0; }
  .big { font-size: 44px; color: var(--faint); }
  .center { justify-content: center; }
  .keys-warn { margin: 6px 12px 0; padding: 8px 10px; border-radius: 6px; font-size: 12px; line-height: 1.4;
    background: var(--warn-bg); color: var(--warn); }
</style>
