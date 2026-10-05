<script lang="ts">
  import { onMount } from "svelte";
  import { Events } from "@wailsio/runtime";
  import { api, errorText, formatBytes, progressShort, type Overview, type Progress, type ProjectInfo, type TeamProject } from "./lib/api";
  import type { DownloadSize, UpdateInfo, UpdateState } from "../bindings/dawgit/desktop/models";
  import ProgressBar from "./lib/ProgressBar.svelte";
  import { toast } from "./lib/notify.svelte";
  import ProjectView from "./lib/ProjectView.svelte";
  import KeptSamples from "./lib/KeptSamples.svelte";
  import Onboarding from "./lib/Onboarding.svelte";
  import TeamMenu from "./lib/TeamMenu.svelte";
  import VerifyDialog from "./lib/VerifyDialog.svelte";
  import Modal from "./lib/Modal.svelte";
  import Toasts from "./lib/Toasts.svelte";
  import ProjectSettings from "./lib/ProjectSettings.svelte";
  import AppSettings from "./lib/AppSettings.svelte";
  import { t, tn } from "./lib/i18n.svelte";

  let overview = $state<Overview | null>(null);
  let onboarding = $state(false);
  // Selected sidebar entry: a project folder, or a team project not here yet.
  let selected = $state<{ root?: string; id?: string }>({});
  let refreshKey = $state(0);
  let busy = $state("");
  let autostart = $state(false);
  let rowMenu = $state(""); // key of the project whose ⋯ menu is open
  let confirmDelete = $state<TeamProject | null>(null);
  // A project about to leave DAWGit: teammates' samples are offered into it first.
  let leaving = $state<{ roots: string[]; go: () => void } | null>(null);
  let checking = $state<TeamProject | null>(null); // Check project…
  let deleteWord = $state("");
  const rowKey = (p: TeamProject) => p.root || p.id;
  let settingsFor = $state<TeamProject | null>(null); // Project settings
  // Closes the settings and gives their project: read it first (what the
  // dialog shows goes with settingsFor).
  function closeSettings(): TeamProject {
    const p = settingsFor!;
    settingsFor = null;
    return p;
  }
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
    if (rowMenu && p.root && p.status === "downloaded") {
      api.ProjectInfo(p.root).then((i) => { if (rowMenu === key) menuInfo = i; }).catch(() => {});
    }
  }
  const toolName = (tool: string) => (tool === "Ableton Live" ? "Live" : tool ? t(tool) : t("its program"));
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
      // At first from this computer alone, at once: asking the team can
      // take a while (offline, storage down), and it fills in after.
      if (!overview) overview = await api.LocalOverview();
      overview = await api.Overview();
      if (overview?.proImported) {
        // Once: DAWGit Pro's teams came over (it becomes the Nightly channel).
        toast(t("DAWGit Pro's teams and projects are here now, on the Nightly channel. You can uninstall DAWGit Pro (Windows Settings › Apps)."), "info", 15000);
        api.DismissProNotice().catch(() => {});
      }
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
  let justDownloaded = $state(""); // show its check when it opens
  let appVersion = $state("");
  let edition = $state(""); // a build with extensions, e.g. "Pro"

  // A newer release: offered until the user hides that version (unless it's
  // required: then the app can't be used before updating). A signed one is
  // downloaded in the background and installed by DAWGit itself.
  let update = $state<UpdateInfo | null>(null);
  let updState = $state<UpdateState>({ stage: "", done: 0, total: 0, error: "", auto: true });
  let installWhenReady = $state(false); // "Update now" before the download finished
  const DISMISSED_KEY = "dawgit.dismissedUpdate";
  async function checkUpdate() {
    try {
      const u = await api.CheckUpdate();
      update = u && (u.required || u.version !== recall(DISMISSED_KEY)) ? u : null;
      updState = await api.UpdateStatus();
    } catch {
      /* offline: try again later */
    }
  }
  let anyBusy = $derived(Object.keys(activity).length > 0);
  // While it downloads, ask how it goes too (an event can come before the
  // first answer and be overwritten by it).
  $effect(() => {
    if (updState.stage !== "downloading") return;
    const t = setInterval(() => api.UpdateStatus().then(onUpdateState).catch(() => {}), 1000);
    return () => clearInterval(t);
  });
  async function updateNow() {
    if (updState.stage === "ready") {
      try {
        await api.InstallUpdate(); // DAWGit closes, updates and opens again
      } catch (e) {
        toast(errorText(e), "error", 8000);
      }
      return;
    }
    installWhenReady = true;
    api.DownloadUpdate().catch(() => {});
  }
  function onUpdateState(st: UpdateState) {
    updState = st;
    if (st.stage === "ready" && installWhenReady && !anyBusy) {
      installWhenReady = false;
      updateNow();
    }
  }
  const pct = (st: UpdateState) => (st.total > 0 ? Math.round((st.done / st.total) * 100) : 0);
  // The settings' "Check for updates": now, and say when there's none.
  let appSettings = $state(false);
  let checkingNow = $state(false);
  async function checkUpdateNow() {
    checkingNow = true;
    try {
      const u = await api.CheckUpdateNow();
      update = u;
      updState = await api.UpdateStatus();
      if (!u) toast(t("You have the newest version"), "ok");
    } catch (e) {
      toast(errorText(e), "error");
    } finally {
      checkingNow = false;
    }
  }
  async function setAutoUpdate(on: boolean) {
    try {
      await api.SetAutoUpdate(on);
      updState = { ...updState, auto: on };
    } catch (e) {
      toast(errorText(e), "error");
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
  // Every project is a team's (0.9: no projects kept on this computer only).
  let entries = $derived.by(() => {
    const list = overview?.projects ?? [];
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
      entries.find((p) => p.status === "downloaded") ?? entries[0];
    if (pick) select(pick);
  });

  // Where downloads go, and the size of the selected project.
  let downloadDir = $state(recall(DOWNLOAD_DIR_KEY));
  let dlSize = $state<{ id: string; size: DownloadSize | null; error: string } | null>(null);
  // The team's projects not downloaded here are listed as it last listed
  // them while it's being asked or can't be reached: not to be downloaded.
  let teamLocked = $derived(!overview?.teamChecked || !!overview?.teamError);
  $effect(() => {
    const p = selectedEntry;
    const team = overview?.currentTeam;
    if (!p || p.status !== "remote" || !team || teamLocked) return;
    const id = p.id;
    dlSize = { id, size: null, error: "" };
    api.ProjectDownloadSize(team, id)
      .then((size) => { if (dlSize?.id === id) dlSize = { id, size, error: "" }; })
      .catch((e) => { if (dlSize?.id === id) dlSize = { id, size: null, error: errorText(e) }; });
  });

  async function download(p: TeamProject) {
    let parent = downloadDir;
    if (!parent) {
      parent = await api.ChooseFolder(t("Where should downloaded projects go?"));
      if (!parent) return;
      remember(DOWNLOAD_DIR_KEY, parent);
      downloadDir = parent;
    }
    busy = p.id;
    lastProgress = null;
    try {
      const got = await api.DownloadProject(overview!.currentTeam, p.id, parent);
      toast(t("Downloaded “{name}” to {folder}", { name: got.name, folder: got.root }), "ok", 7000);
      justDownloaded = got.root;
      await reload();
      select(got);
    } catch (e) {
      toast(errorText(e), "error", 9000);
    } finally {
      busy = "";
    }
  }

  async function changeDownloadDir() {
    const d = await api.ChooseFolder(t("Where should downloaded projects go?"));
    if (d) {
      remember(DOWNLOAD_DIR_KEY, d);
      downloadDir = d;
    }
  }

  async function locate(p: TeamProject) {
    const folder = await api.ChooseFolder(t("Where is “{name}” now?", { name: p.name }));
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
    toast(t("Unlinked “{name}”: the folder and its versions are untouched", { name: p.name }), "info");
    selected = {};
    await reload();
  }

  function leaveThenDelete(p: TeamProject) {
    confirmDelete = null;
    if (p.root) leaving = { roots: [p.root], go: () => deleteFromTeam(p) };
    else deleteFromTeam(p);
  }

  async function deleteFromTeam(p: TeamProject) {
    confirmDelete = null;
    busy = "delete";
    try {
      await api.DeleteProjectFromTeam(overview!.currentTeam, p.id);
      toast(t(p.root ? "Deleted “{name}” from {team}. Your copy stays in its folder." : "Deleted “{name}” from {team}",
        { name: p.name, team: current?.name ?? "" }), "info", 8000);
      if (!p.root) selected = {};
      await reload();
    } catch (e) {
      toast(errorText(e), "error", 9000);
    } finally {
      busy = "";
    }
  }

  async function addToTeam() {
    // No confirmation here: the project asks before sharing anything.
    const folder = await api.ChooseFolder(t("Choose a project folder to add to {team}", { team: current?.name ?? t("the team") }));
    if (folder) share(folder);
  }

  async function share(folder: string) {
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


  async function toggleAutostart(on: boolean) {
    try {
      await api.SetAutostart(on);
      autostart = on;
    } catch (e) {
      toast(errorText(e), "error");
    }
  }

  type WatchEvent = { root: string; kind: string; author: string; labels: string[]; text: string; versions: { author: string; message: string }[] };

  onMount(() => {
    reload().then(() => {
      if (overview && overview.teams.length === 0) onboarding = true;
    });
    api.Autostart().then((on) => (autostart = on)).catch(() => {});
    api.Version().then((v) => (appVersion = v)).catch(() => {});
    api.Edition().then((e) => (edition = e)).catch(() => {});
    checkUpdate();
    const offProgress = Events.On("progress", (ev: { data: Progress }) => onProgress(ev.data));
    const offUpdate = Events.On("update", (ev: { data: UpdateState }) => onUpdateState(ev.data));
    const offWatch = Events.On("team-watch", (ev: { data: WatchEvent }) => {
      const e = ev.data;
      const name = entries.find((p) => p.root === e.root)?.name ?? "";
      switch (e.kind) {
        case "new-versions":
          toast(`${name}: ${e.versions.map((v) => t("{author} saved “{message}”", { author: v.author, message: v.message })).join("\n")}`, "info", 8000);
          break;
      }
      if (e.root === selected.root) refreshKey++;
    });
    return () => {
      offProgress();
      offUpdate();
      offWatch();
    };
  });

  const statusText = (status: string) => ({ remote: t("not downloaded"), missing: t("folder not found") } as Record<string, string>)[status];
  const statusIcon: Record<string, string> = { remote: "☁", missing: "⚠", downloaded: "♪" };
</script>

<svelte:window onfocus={reloadIfStale}
  onclick={(e) => { if (rowMenu && !(e.target as HTMLElement).closest(".row-menu, .more")) rowMenu = ""; }} />

{#if !overview}
  <div class="splash" role="status" aria-label={t("Loading")}>
    <div class="splash-logo"><img src="/icon.png" alt="" />DAWGit</div>
    <div class="splash-band"></div>
  </div>
{:else if onboarding}
  <Onboarding {overview} {reload} onfinish={async (root, share) => {
    onboarding = false;
    if (root && share) firstShare = root;
    else if (root) justDownloaded = root;
    await reload();
    if (root) selected = { root };
  }} />
{:else}
  <div class="shell">
    <aside>
      <button class="brand" onclick={() => (appSettings = true)} title={t("DAWGit settings")}>
        <img src="/icon.png" alt="" /> DAWGit
        {#if edition}<span class="edition" title={t("A DAWGit build with extensions")}>{edition}</span>{/if}
        {#if appVersion}<span class="version faint">v{appVersion}</span>{/if}
      </button>
      {#if update}
        {@const u = update}
        <div class="update">
          <div class="update-h">
            <span>{t("{app} {version} is available", { app: `DAWGit${edition ? ` ${edition}` : ""}`, version: u.version })}</span>
            {#if !u.required}
              <button class="ghost x" title={t("Hide until the next version")}
                onclick={() => { remember(DISMISSED_KEY, u.version); update = null; }}>✕</button>
            {/if}
          </div>
          {@render updateActions(u)}
        </div>
      {/if}
      <TeamMenu {overview} {reload} />
      {#if current?.keysUnreadable}
        <p class="keys-warn">{t("This computer can't read the keys of “{team}” (DAWGit's settings came from another computer or Windows user). Enter them again in the team's settings (⚙).", { team: current.name })}</p>
      {/if}

      <div class="list">
        <div class="section row-h">
          <span>{t("Projects")}</span>
          {#if current}
            <button class="ghost tiny" class:spin={reloading} onclick={reload} title={t("Check the team for new projects")}>↻</button>
          {/if}
        </div>
        {#if current && overview.teamError}
          <div class="offline" title={overview.teamError}>● {current.isStorage ? t("Storage not reachable") : t("Server not reachable")}</div>
        {:else if current && !overview.teamChecked}
          <div class="offline checking">● {t("checking…")}</div>
        {/if}
        <ul>
          {#each entries as p (p.root || p.id)}
            {@render row(p)}
          {:else}
            <li class="empty faint">{t("No projects in this team yet.")}</li>
          {/each}
        </ul>
        <button class="add" onclick={addToTeam} disabled={busy === "add" || !current}>
          <span>+ {t("Add project")}</span>
          <span class="hint">{t("Select project folder")}</span>
        </button>
      </div>

    </aside>

    <section class="content">
      {#if selectedEntry && selectedEntry.status === "downloaded"}
        {#key selectedEntry.root}
          <ProjectView root={selectedEntry.root} {refreshKey} teams={overview.teams} onchanged={reload}
            onsettings={() => (settingsFor = selectedEntry ?? null)}
            firstShare={firstShare === selectedEntry.root} onfirstshared={() => (firstShare = "")}
            downloaded={justDownloaded === selectedEntry.root} ondownloadseen={() => (justDownloaded = "")} />
        {/key}
      {:else if selectedEntry && selectedEntry.status === "remote"}
        {@const p = selectedEntry}
        <div class="placeholder">
          <div class="big" aria-hidden="true">☁</div>
          <h1>{p.name}</h1>
          <p class="muted">{t("This project is on {team} but not on this computer yet.", { team: current?.name ?? "" })}</p>
          {#if teamLocked}
            <p class="faint">{overview.teamError ? t("{team} can't be reached right now: download it once it's back.", { team: current?.name ?? "" })
              : t("Checking {team}…", { team: current?.name ?? "" })}</p>
          {:else if dlSize?.id === p.id && dlSize.size}
            {@const z = dlSize.size}
            <p class="size">{formatBytes(z.bytes)} · {tn(z.files, "{count} file", "{count} files", { count: z.files.toLocaleString() })}</p>
          {:else if dlSize?.id === p.id && !dlSize.error}
            <p class="size faint">{t("Checking the size…")}</p>
          {/if}
          <button class="primary" onclick={() => download(p)} disabled={!!busy || teamLocked}>
            {busy === p.id ? t("Downloading…") : `↓ ${t("Download")}`}
          </button>
          {#if busy === p.id && lastProgress}
            <div class="dl-progress"><ProgressBar p={lastProgress} /></div>
          {/if}
          <p class="faint small">{t("Into {folder}", { folder: downloadDir || t("a folder you choose") })} ·
            <button class="link" onclick={changeDownloadDir}>{t("change")}</button></p>
        </div>
      {:else if selectedEntry && selectedEntry.status === "missing"}
        {@const p = selectedEntry}
        <div class="placeholder">
          <div class="big" aria-hidden="true">⚠</div>
          <h1>{p.name}</h1>
          <p class="muted">{t("The project folder was moved or deleted:")}<br /><span class="mono">{p.root}</span></p>
          <div class="row center">
            <button class="primary" onclick={() => locate(p)}>{t("Locate folder…")}</button>
            <button onclick={() => download(p)} disabled={!!busy}>{t("Download again")}</button>
            <button class="ghost" onclick={() => forget(p)} title={t("DAWGit stops listing it; nothing is deleted")}>{t("Unlink folder")}</button>
          </div>
        </div>
      {:else}
        <div class="placeholder">
          <h1>{current?.name ?? ""}</h1>
          <p class="muted">{t("Pick a project on the left, or add one to share it with the team.")}</p>
        </div>
      {/if}
    </section>
  </div>
{/if}

{#snippet updateActions(u: UpdateInfo)}
  <div class="update-a">
    {#if !u.installable}
      {#if u.downloadUrl}
        <button class="primary" onclick={() => openLink(u.downloadUrl)}
          title={t("Download the installer; run it to update (your projects and teams are kept)")}>{t("Download")}</button>
      {/if}
    {:else if updState.stage === "downloading" || (installWhenReady && updState.stage !== "failed")}
      <span class="upd-progress"><span style:width="{pct(updState)}%"></span></span>
      <span class="faint small">{updState.stage === "ready" ? t("Installing…") : t("Downloading {percent}%", { percent: pct(updState) })}</span>
    {:else if updState.stage === "ready"}
      <button class="primary" onclick={updateNow} disabled={anyBusy}
        title={anyBusy ? t("After the project's current upload or download") : t("DAWGit closes, updates and opens again (your projects and teams are kept)")}>
        {t("Restart to update")}</button>
    {:else}
      <button class="primary" onclick={updateNow} disabled={anyBusy}
        title={t("Downloads it, then DAWGit closes, updates and opens again")}>{t("Update now")}</button>
    {/if}
    <button class="ghost" onclick={() => openLink(u.pageUrl)}>{t("What's new")}</button>
  </div>
  {#if updState.stage === "failed"}<p class="upd-error">{updState.error}</p>{/if}
  {#if u.installable && updState.stage === "ready" && updState.auto && !u.required}
    <p class="faint small upd-note">{t("Or it installs by itself when DAWGit is in the tray.")}</p>
  {/if}
{/snippet}

{#snippet row(p: TeamProject)}
  <li>
    <button class="proj {p.status}" class:on={selectedEntry === p} onclick={() => select(p)}
      class:locked={p.status === "remote" && teamLocked}
      title={p.status === "remote" ? t("On the team, not on this computer yet") : p.root}>
      <span class="icon" aria-hidden="true">{statusIcon[p.status]}</span>
      <span class="text">
        <span class="name">{p.name}{#if pinned.includes(rowKey(p))}<svg class="pin" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-label={t("Pinned")}><title>{t("Pinned")}</title><path d="M12 17v5"/><path d="M9 10.76a2 2 0 0 1-1.11 1.79l-1.78.9A2 2 0 0 0 5 15.24V16a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1v-.76a2 2 0 0 0-1.11-1.79l-1.78-.9A2 2 0 0 1 15 10.76V7a1 1 0 0 1 1-1 2 2 0 0 0 0-4H8a2 2 0 0 0 0 4 1 1 0 0 1 1 1z"/></svg>{/if}</span>
        <span class="meta" class:busy={p.root && activity[p.root]}>
          {p.root && activity[p.root] ? progressShort(activity[p.root]) : statusText(p.status) ?? `⑂ ${p.branch}`}
        </span>
      </span>
    </button>
    <button class="ghost more" class:open={rowMenu === rowKey(p)} title={t("More")}
      onclick={() => toggleMenu(p)}>⋯</button>
    {#if rowMenu === rowKey(p)}
      <div class="row-menu" role="menu">
        <button class="item" onclick={() => { rowMenu = ""; togglePin(p); }}>{pinned.includes(rowKey(p)) ? t("Unpin") : t("Pin to top")}</button>
        {#if menuInfo && menuInfo.openable.length === 1}
          <button class="item" onclick={() => openIn(p, menuInfo!.openable[0])}>{t("Open in {tool}", { tool: toolName(menuInfo.tool) })}</button>
        {:else if menuInfo && menuInfo.openable.length > 1}
          <div class="sub" role="none" onmouseenter={showSub} onmouseleave={hideSub}>
            <button class="item has-sub" aria-expanded={openSub}>
              {t("Open in {tool}", { tool: toolName(menuInfo.tool) })}<span class="arrow">›</span>
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
          <button class="item" onclick={() => { rowMenu = ""; api.ShowFolder(p.root); }}>{t("Open folder")}</button>
        {/if}
        <div class="sep"></div>
        <button class="item" onclick={() => { rowMenu = ""; settingsFor = p; }}>{t("Settings…")}</button>
      </div>
    {/if}
  </li>
{/snippet}


{#if appSettings}
  <AppSettings version={appVersion} {edition} {autostart} autoUpdate={updState.auto} {downloadDir} {update}
    checking={checkingNow} onautostart={toggleAutostart} onautoupdate={setAutoUpdate} oncheck={checkUpdateNow}
    ondownloaddir={changeDownloadDir} onupdate={(u) => (update = u)} onclose={() => (appSettings = false)} />
{/if}

{#if settingsFor}
  {@const p = settingsFor}
  <ProjectSettings {p} team={current}
    onclose={() => (settingsFor = null)}
    onrenamed={async () => { const key = rowKey(p); await reload(); refreshKey++; settingsFor = entries.find((e) => rowKey(e) === key) ?? null; }}
    oncheck={() => { checking = closeSettings(); }}
    ondelete={() => { const q = closeSettings(); deleteWord = ""; confirmDelete = q; }}
    onunlink={() => { const q = closeSettings(); leaving = { roots: [q.root], go: () => forget(q) }; }}
    onlocate={() => locate(closeSettings())} />
{/if}

{#if checking}
  <VerifyDialog root={checking.root} name={checking.name} onclose={() => (checking = null)} />
{/if}


{#if confirmDelete}
  {@const p = confirmDelete}
  <Modal title={t("Delete “{name}” from the team?", { name: p.name })} onclose={() => (confirmDelete = null)}>
    <p>{t(p.root ? "This removes the project and all its versions from {team}, for everyone in the team. Copies already on someone's computer are not touched — yours stays in its folder."
      : "This removes the project and all its versions from {team}, for everyone in the team. Copies already on someone's computer are not touched.", { team: current?.name ?? "" })}</p>
    <label for="delete-word">{t("Type {name} to confirm", { name: p.name })}</label>
    <input id="delete-word" class="confirm-input" bind:value={deleteWord} autocomplete="off"
      onkeydown={(e) => { if (e.key === "Enter" && deleteWord.trim() === p.name) leaveThenDelete(p); }} />
    {#snippet footer()}
      <button onclick={() => (confirmDelete = null)}>{t("Cancel")}</button>
      <button class="danger" disabled={deleteWord.trim() !== p.name} onclick={() => leaveThenDelete(p)}>{t("Delete")}</button>
    {/snippet}
  </Modal>
{/if}

{#if leaving}
  <!-- go is taken before leaving is cleared: an {@const} of it would clear too. -->
  <KeptSamples roots={leaving.roots} oncancel={() => (leaving = null)}
    onproceed={() => { const go = leaving?.go; leaving = null; go?.(); }} />
{/if}

{#if update?.required}
  {@const u = update}
  <div class="must-update" role="dialog" aria-modal="true" aria-label={t("Update DAWGit")}>
    <div class="must-card">
      <h2>{t("Update DAWGit to keep going")}</h2>
      <p class="muted">{t("This version of DAWGit can no longer work with your team's projects: they need DAWGit {version}. Updating takes a minute, and keeps your projects and teams.", { version: u.version })}</p>
      {@render updateActions(u)}
    </div>
  </div>
{/if}

<Toasts />

<style>
  .shell { display: grid; grid-template-columns: 250px 1fr; height: 100%; }
  aside { background: #141518; border-right: 1px solid var(--line); display: flex; flex-direction: column; padding: 12px 10px; min-height: 0; }
  .brand { display: flex; align-items: center; gap: 8px; font-weight: 700; font-size: 15px; padding: 4px 8px; margin: -2px 0 8px;
    background: none; border: 0; border-radius: 6px; color: var(--text); text-align: left; cursor: pointer; width: 100%; }
  .brand:hover { background: var(--panel-2); }
  .brand img { width: 20px; height: 20px; }
  .list { flex: 1; overflow: auto; min-height: 0; }
  .section { font-size: 11px; text-transform: uppercase; letter-spacing: .06em; color: var(--faint); padding: 8px 8px 4px; }
  .row-h { display: flex; align-items: center; justify-content: space-between; }
  .tiny { padding: 0 6px; font-size: 13px; line-height: 18px; color: var(--faint); }
  .tiny:hover { color: var(--text); }
  .spin { animation: spin .8s linear infinite; }
  @keyframes spin { to { transform: rotate(360deg); } }
  .meta.busy { color: var(--accent); }
  .size { margin: 0 0 12px; font-size: 13px; }
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
  .proj.locked { opacity: .6; }
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
  .danger-text { color: var(--danger); }
  .confirm-input { width: 100%; margin-top: 6px; }
  .empty { padding: 6px 10px; font-size: 13px; }
  .offline { font-size: 12px; color: var(--danger); padding: 0 8px 4px; }
  .offline.checking { color: var(--faint); }
  .add { width: 100%; margin: 8px 0 4px; display: flex; flex-direction: column; align-items: center; gap: 0; padding: 5px 10px; line-height: 1.3; }
  .add .hint { font-size: 11px; color: var(--faint); font-weight: 400; }
  .pad { padding: 0 8px; }
  .link { border: none; background: none; color: var(--muted); text-decoration: underline; padding: 0; font-size: 12.5px; text-align: left; }
  .version { margin-left: auto; font-size: 11px; font-weight: 400; }
  .edition { font-size: 10px; font-weight: 700; letter-spacing: .06em; text-transform: uppercase; padding: 1px 6px;
    border-radius: 6px; color: var(--accent-ink); background: var(--accent); }
  .update { margin: 0 0 10px; padding: 8px 10px; border-radius: 8px; background: #1f3b35; border: 1px solid #2c5a4e; font-size: 12.5px; }
  .update-h { display: flex; align-items: center; gap: 6px; font-weight: 600; color: var(--accent); }
  .update-h span { flex: 1; }
  .update-a { display: flex; gap: 6px; margin-top: 6px; }
  .update-a button { padding: 3px 10px; font-size: 12.5px; }
  .update-a { align-items: center; }
  .upd-progress { flex: 1; height: 5px; border-radius: 3px; background: #2c5a4e; overflow: hidden; }
  .upd-progress span { display: block; height: 100%; background: var(--accent); transition: width .2s; }
  .upd-error { margin: 6px 0 0; color: var(--danger); font-size: 12px; }
  .upd-note { margin: 5px 0 0; }
  .must-update { position: fixed; inset: 0; z-index: 100; display: flex; align-items: center; justify-content: center;
    background: rgba(8, 9, 11, .82); }
  .must-card { width: 440px; max-width: calc(100vw - 48px); padding: 22px 24px; border-radius: 12px;
    background: var(--panel-2); border: 1px solid var(--line); }
  .must-card h2 { margin: 0 0 8px; font-size: 18px; }
  .must-card .update-a button { padding: 6px 14px; font-size: 13.5px; }
  .x { padding: 0 5px; line-height: 16px; color: var(--muted); }
  .small { font-size: 12.5px; }
  .content { min-width: 0; overflow: hidden; }
  .placeholder { height: 100%; display: flex; flex-direction: column; align-items: center; justify-content: center; text-align: center; padding: 40px; }
  .placeholder h1 { margin: 6px 0; }
  .big { font-size: 44px; color: var(--faint); }
  .center { justify-content: center; }
  .keys-warn { margin: 6px 12px 0; padding: 8px 10px; border-radius: 6px; font-size: 12px; line-height: 1.4;
    background: var(--warn-bg); color: var(--warn); }
</style>
