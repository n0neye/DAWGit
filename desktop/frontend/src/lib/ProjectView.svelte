<script lang="ts">
  import { untrack } from "svelte";
  import { Events } from "@wailsio/runtime";
  import { api, ago, errorText, type State, type Result, type Preview, type Conflict, type TeamSummary,
    type Progress, type Version } from "./api";
  import { toast } from "./notify.svelte";
  import { cachedState, rememberState } from "./stateCache";
  import ChangesPanel from "./ChangesPanel.svelte";
  import CombineDialog from "./CombineDialog.svelte";
  import ProgressBar from "./ProgressBar.svelte";
  import History from "./History.svelte";
  import Modal from "./Modal.svelte";
  import PreviewDialog from "./PreviewDialog.svelte";
  import ConflictDialog from "./ConflictDialog.svelte";

  // firstShare: the project was just added to a team; commit and upload its
  // first version right away.
  let { root, refreshKey, teams, firstShare = false, onchanged, onfirstshared }: {
    root: string; refreshKey: number; teams: TeamSummary[]; firstShare?: boolean;
    onchanged: () => void; onfirstshared?: () => void;
  } = $props();

  let st = $state<State | null>(cachedState(untrack(() => root)));
  let loadError = $state("");
  // The tab is remembered per project.
  type Tab = "changes" | "history";
  const tabKey = `dawgit.tab:${untrack(() => root)}`;
  let tab = $state<Tab>((() => {
    try {
      const t = localStorage.getItem(tabKey);
      return t === "history" ? t : "changes";
    } catch {
      return "changes";
    }
  })());
  $effect(() => {
    const t = tab;
    try { localStorage.setItem(tabKey, t); } catch { /* not remembered */ }
  });
  let message = $state("");
  let busy = $state("");
  let progress = $state<Progress | null>(null);
  let refreshing = $state(false);

  // dialogs
  let preview = $state<{ title: string; label: string; data: Preview; run: Action; blocked: string } | null>(null);
  const UNSAVED = "You have uncommitted changes. Commit a version instead — the team's changes are merged in as part of it, and anything you both changed is shown then.";
  let conflicts = $state<{ items: Conflict[]; run: Action; force: boolean } | null>(null);
  let liveBlocked = $state<{ run: Action; resolutions: Record<string, string>; set: string } | null>(null);
  let shareOpen = $state(false);
  let shareTeam = $state("");
  let branchMenu = $state(false);
  let setMenu = $state(false);
  let newBranch = $state<string | null>(null);
  // Go to version: asked first when there are uncommitted changes.
  let leaving = $state<{ target: Version | null; message: string } | null>(null); // null target: latest
  let keepOpen = $state<string | null>(null); // message for "Make this the latest version"

  // message: for a save, what to describe the version with (asked again when
  // the team committed in the meantime: see combine).
  type Action = { name: string; call: (res: Record<string, string>, force: boolean) => Promise<Result | null>;
    done: (r: Result) => void; message?: string };

  // Teammates committed on this branch while you were working: preview, then
  // combine, put your work on a branch, or discard it.
  let combine = $state<{ data: Preview; message: string } | null>(null);
  let branchThenCommit = $state(""); // commit this after creating the branch
  let discardOpen = $state(false);
  let discardFile = $state(""); // one file's changes, after confirming
  let discardAllOpen = $state(false);
  let restoreFile = $state<{ path: string; version: string; label: string } | null>(null);

  // Two steps: the project folder (fast), then the team's side (network),
  // so the page never waits for the team.
  let teamLoading = false;
  async function load() {
    const r = root;
    let local: State;
    try {
      local = (await api.State(r))!;
      st = local;
      rememberState(local);
      loadError = "";
    } catch (e) {
      loadError = errorText(e);
      return;
    }
    if (!local.remoteUrl || teamLoading) return;
    teamLoading = true;
    try {
      const t = await api.TeamState(r);
      if (t && st?.root === r) {
        st = { ...st, ...t, teamChecked: true } as State;
        rememberState(st);
      }
    } catch {
      // shown as "not reachable" by the next round
    } finally {
      teamLoading = false;
    }
  }

  let firstShareStarted = false;
  $effect(() => {
    root; refreshKey;
    if (untrack(() => firstShare) && !firstShareStarted) {
      firstShareStarted = true;
      shareFirstVersion();
      return;
    }
    load();
  });

  // The team's side (new versions, branches) is refreshed every minute (and
  // when the agent reports new versions)...
  $effect(() => {
    const t = setInterval(() => { if (!busy) load(); }, 60000);
    return () => clearInterval(t);
  });

  // ...but a Ctrl+S in Live shows up within a couple of seconds: the set's
  // size and time are checked every second, and the changes are read once the
  // file has stopped changing.
  $effect(() => {
    const r = root;
    let sig = "", pending = "";
    const t = setInterval(async () => {
      if (busy) return;
      let s: string;
      try {
        s = await api.Signature(r);
      } catch {
        return;
      }
      if (!sig || s === sig) {
        sig = s;
        pending = "";
      } else if (s !== pending) {
        pending = s;
      } else {
        sig = s;
        pending = "";
        load();
      }
    }, 1000);
    return () => clearInterval(t);
  });

  // Other files (samples added, converted, edited elsewhere) show up as soon
  // as the folder watcher sees them.
  $effect(() => {
    const r = root;
    api.WatchFiles(r);
    const off = Events.On("files", (ev: { data: { root: string } }) => {
      if (ev.data.root === r && !busy) load();
    });
    return () => {
      off();
      api.UnwatchFiles(r);
    };
  });

  $effect(() => {
    const r = root;
    let timer: ReturnType<typeof setTimeout>;
    const off = Events.On("progress", (ev: { data: Progress }) => {
      if (ev.data.root !== r) return;
      progress = ev.data.stage === "done" ? null : ev.data;
      // In case the final event is missed (e.g. the window reloaded).
      clearTimeout(timer);
      timer = setTimeout(() => (progress = null), 10 * 60000);
    });
    return () => {
      off();
      clearTimeout(timer);
    };
  });

  async function refresh() {
    refreshing = true;
    await load();
    refreshing = false;
  }

  let incomingIds = $derived(new Set(st?.incoming.map((v) => v.id) ?? []));

  // Runs an action; handles conflicts (ask, retry with decisions) and a
  // running Live (ask, retry with force).
  async function run(a: Action, resolutions: Record<string, string> = {}, force = false) {
    busy = a.name;
    try {
      const r = await a.call(resolutions, force);
      if (!r) return;
      if (r.action === "behind") {
        openCombine(a.message ?? message); // nothing changed yet: let the user decide
      } else if (r.liveRunning) {
        liveBlocked = { run: a, resolutions, set: r.openSet };
      } else if (r.conflicts.length) {
        conflicts = { items: r.conflicts, run: a, force }; // keep a "Live is closed" confirmation
      } else {
        a.done(r);
        if (r.relinked.length) toast(`Relinked ${r.relinked.length} sample path(s) for this computer`, "info");
      }
    } catch (e) {
      toast(errorText(e), "error", 9000);
    } finally {
      busy = "";
      progress = null;
      await load();
      onchanged();
    }
  }

  const saveDone = (r: Result) => {
      const text: Record<string, string> = {
        "published": "Version committed and shared with the team",
        "saved-locally": "Version committed on this computer (not shared with a team)",
        "fast-forward": "You had nothing new; updated to the team's latest version",
        "nothing": "Nothing changed since your last version",
      };
      toast(text[r.action] ?? "Version committed", r.action === "nothing" ? "info" : "ok");
      if (r.log.length && r.action === "published") toast("The team's changes were merged into your files" + reopen(), "warn", 9000);
      if (r.action !== "nothing") message = "";
  };

  const saveAction = (msg: string, combineWithTeam = false): Action => ({
    name: "save",
    message: msg,
    call: (res, force) => api.Save(root, msg, combineWithTeam, res, force),
    done: saveDone,
  });

  // Commit from the commit box: when the team is ahead, ask first.
  async function commit(confirmed = false) {
    if (!message.trim() || busy || (st?.olderVersion && !st.remoteUrl)) return;
    if (!confirmed) {
      // Files the rules now leave out, and what the project's checks warn
      // about (e.g. a Unity asset without its .meta): say so first.
      const leaving = st?.changes.filter((c) => c.status === "untracked").map((c) => c.path) ?? [];
      const warnings = (await api.CommitWarnings(root).catch(() => [])) ?? [];
      if (leaving.length || warnings.length) {
        untrackedConfirm = leaving;
        commitWarnings = warnings;
        return;
      }
    }
    if (st?.incoming.length || st?.olderVersion) openCombine(message);
    else run(saveAction(message));
  }

  // The tool the project is made with: Live gets its own words.
  let isLive = $derived(st?.tool === "Ableton Live");
  // What to do after DAWGit changed the project's files.
  const reopen = () => (st?.tool === "Ableton Live" ? " — reopen the set in Live"
    : st?.tool ? ` — switch back to ${st.tool} to load the changes` : "");
  const label = (rel: string) => (rel === "." ? st?.name ?? "the project" : rel);

  // The project's rules (.dawgit.yaml): files no longer tracked, the dialog.
  let untrackedConfirm = $state<string[] | null>(null);
  let commitWarnings = $state<string[]>([]);
  let rulesOpen = $state(false);
  async function openRules() {
    try {
      await api.OpenRules(root);
      toast("Save the file, then DAWGit follows the new rules", "info");
    } catch (e) {
      toast(errorText(e), "error");
    }
  }

  async function openCombine(msg: string) {
    busy = "preview";
    try {
      const data = await api.PreviewUpdate(root);
      if (data) combine = { data, message: msg };
    } catch (e) {
      toast(errorText(e), "error");
    } finally {
      busy = "";
    }
  }

  function combineAndShare() {
    const msg = combine!.message.trim();
    combine = null;
    message = msg;
    run(saveAction(msg, true));
  }

  function putOnBranch(msg: string) {
    combine = null;
    message = msg.trim();
    branchThenCommit = msg.trim();
    newBranch = "";
  }

  function discardEverything() {
    discardAllOpen = false;
    run({
      name: "discard",
      call: (_res, force) => api.DiscardAll(root, force),
      done: () => toast("Discarded all your uncommitted changes" + reopen(), "ok"),
    });
  }

  function restoreOneFile() {
    const r = restoreFile!;
    restoreFile = null;
    run({
      name: "restore",
      call: (_res, force) => api.RestoreFileVersion(root, r.path, r.version, force),
      done: () => toast(`Restored ${r.path.slice(r.path.lastIndexOf("/") + 1)} from “${r.label}” — commit it to keep it`, "ok", 8000),
    });
  }

  function discardOneFile() {
    const path = discardFile;
    discardFile = "";
    run({
      name: "discard",
      call: (_res, force) => api.DiscardFile(root, path, force),
      done: () => toast(`Discarded your changes to ${path.slice(path.lastIndexOf("/") + 1)}`, "ok"),
    });
  }

  function discardAndUpdate() {
    discardOpen = false;
    run({
      name: "update",
      call: (res, force) => api.DiscardAndUpdate(root, res, force),
      done: () => toast("Your changes were discarded and you have the team's latest versions" + reopen(), "ok", 8000),
    });
  }

  const updateAction: Action = {
    name: "update",
    call: (res, force) => api.Update(root, res, force),
    done: (r) => {
      if (r.action === "fast-forward" || r.action === "merged") {
        toast("You're up to date" + reopen(), "ok", 8000);
        if (r.action === "merged") toast("Your versions and the team's were combined. Commit a version to share the result.", "info", 9000);
      } else toast("Already up to date", "info");
    },
  };

  async function openUpdatePreview() {
    busy = "preview";
    try {
      const data = await api.PreviewUpdate(root);
      mergeMessage = null;
      if (data) preview = { title: "Updates from the team", label: "Get updates", data, run: updateAction,
        blocked: st?.changes.length ? UNSAVED : "" };
    } catch (e) {
      toast(errorText(e), "error");
    } finally {
      busy = "";
    }
  }

  // The merge version's description, edited in the preview (null: no merge).
  let mergeMessage = $state<string | null>(null);

  async function openMergePreview(name: string) {
    branchMenu = false;
    busy = "preview";
    try {
      const data = await api.PreviewMerge(root, name);
      if (!data) return;
      mergeMessage = data.message;
      preview = {
        title: `Merge “${name}” into “${st?.branch}”`, label: "Merge and share", data,
        blocked: st?.changes.length ? "You have uncommitted changes. Commit a version first, then merge." : "",
        run: {
          name: "merge",
          call: (res, force) => api.MergeBranch(root, name, mergeMessage ?? "", res, force),
          done: (r) => toast(r.action === "up-to-date" || r.action === "ahead"
            ? `Nothing to merge from ${name}` : `Merged ${name} into ${st?.branch} and shared it${reopen()}`, "ok", 8000),
        },
      };
    } catch (e) {
      toast(errorText(e), "error");
    } finally {
      busy = "";
    }
  }

  // Merge any version (e.g. one in the middle of another branch) into the
  // current branch.
  async function openVersionMerge(v: Version) {
    const label = v.branches.length ? v.branches[0] : `“${v.message || v.short}”`;
    busy = "preview";
    try {
      const data = await api.PreviewMergeVersion(root, v.id);
      if (!data) return;
      mergeMessage = data.message;
      preview = {
        title: `Merge ${label} into “${st?.branch}”`, label: "Merge and share", data,
        blocked: st?.changes.length ? "You have uncommitted changes. Commit a version first, then merge." : "",
        run: {
          name: "merge",
          call: (res, force) => api.MergeVersion(root, v.id, mergeMessage ?? "", res, force),
          done: (r) => toast(r.action === "up-to-date" || r.action === "ahead"
            ? `“${st?.branch}” already has ${label}` : `Merged ${label} into ${st?.branch} and shared it${reopen()}`, "ok", 8000),
        },
      };
    } catch (e) {
      toast(errorText(e), "error");
    } finally {
      busy = "";
    }
  }

  function switchTo(name: string) {
    branchMenu = false;
    run({
      name: "switch",
      call: (_res, force) => api.SwitchBranch(root, name, force),
      done: () => toast(`Now working on “${name}”${reopen()}`, "ok", 8000),
    });
  }

  // --- versions: go to, back to latest, keep, export ---

  const goAction = (id: string, discard: boolean, label: string): Action => ({
    name: "goto",
    call: (_res, force) => api.GoToVersion(root, id, discard, force),
    done: () => toast(id === "latest" ? "Back to the latest version" + reopen()
      : `Now on “${label}”${reopen()}`, "ok", 8000),
  });

  // Go to a version (null: back to the latest), asking first about
  // uncommitted changes.
  function goTo(v: Version | null) {
    if (st?.changes.length) {
      leaving = { target: v, message: "" };
      return;
    }
    run(goAction(v?.id ?? "latest", false, v?.message ?? ""));
  }

  async function commitThenGo() {
    const l = leaving!;
    leaving = null;
    let committed = false;
    await run({
      name: "save",
      message: l.message,
      call: (res, force) => api.Save(root, l.message, false, res, force),
      done: () => (committed = true),
    });
    if (committed) run(goAction(l.target?.id ?? "latest", false, l.target?.message ?? ""));
  }

  function discardThenGo() {
    const l = leaving!;
    leaving = null;
    run(goAction(l.target?.id ?? "latest", true, l.target?.message ?? ""));
  }

  function keepThisVersion() {
    const msg = (keepOpen ?? "").trim();
    keepOpen = null;
    run({
      name: "keep",
      call: (res) => api.KeepThisVersion(root, msg, res),
      done: () => toast("This version is now the latest", "ok"),
    });
  }

  async function exportVersion(v: Version) {
    const parent = await api.ChooseFolder("Where should the copy of this version go?");
    if (!parent) return;
    busy = "export";
    try {
      const dir = await api.ExportVersion(root, v.id, parent);
      toast(`Saved a copy of “${v.message || v.short}” as ${dir}`, "ok", 9000);
      api.ShowFolder(dir);
    } catch (e) {
      toast(errorText(e), "error", 9000);
    } finally {
      busy = "";
      progress = null;
    }
  }

  async function createBranch() {
    const name = (newBranch ?? "").trim();
    if (!name) return;
    busy = "branch";
    try {
      await api.CreateBranch(root, name);
      newBranch = null;
      const msg = branchThenCommit;
      branchThenCommit = "";
      if (msg) {
        busy = "";
        await run({ ...saveAction(msg), done: (r) => { saveDone(r);
          toast(`Your work is on the new branch “${name}”; “${st?.branch}” is unchanged. Merge it when you're ready.`, "info", 9000); } });
        return;
      }
      toast(`Created “${name}”. Versions you save now go there.`, "ok");
      await load();
      onchanged();
    } catch (e) {
      toast(errorText(e), "error");
    } finally {
      busy = "";
    }
  }

  // A project kept on this computer only can be moved into a team.
  function openShare() {
    shareTeam = teams[0]?.id ?? "";
    shareOpen = true;
  }

  async function shareWithTeam() {
    const teamId = shareTeam;
    shareOpen = false;
    try {
      await api.ShareProject(root, teamId);
    } catch (e) {
      toast(errorText(e), "error", 9000);
      return;
    }
    onchanged(); // the project moves to the team's list
    shareFirstVersion(teams.find((t) => t.id === teamId)?.name);
  }

  // Commits and uploads the first version of a project that just joined a
  // team.
  function shareFirstVersion(team?: string) {
    onfirstshared?.(); // started: don't start again if this view is reopened
    run({
      name: "first-share",
      call: (res, force) => api.Save(root, "First version", true, res, force),
      done: () => {
        toast(`“${st?.name ?? folderName}” is shared with ${team || st?.teamName || "the team"}`, "ok");
      },
    });
  }

  let folderName = $derived(root.split(/[\\/]/).pop()?.replace(/ Project$/, "") ?? root);
</script>

<svelte:window onfocus={() => { if (!busy) load(); }} onclick={(e) => {
  const t = e.target as HTMLElement;
  if (branchMenu && !t.closest(".branch-wrap")) branchMenu = false;
  if (setMenu && !t.closest(".open-wrap")) setMenu = false;
}} />

{#if loadError && !st && !busy}
  <div class="pad"><p class="error">{loadError}</p></div>
{:else if !st}
  <div class="preparing">
    <h1>{folderName}</h1>
    {#if busy === "first-share"}
      <p class="muted">Sharing with the team: DAWGit commits a first version and uploads it, samples included.
        Large projects can take a few minutes — you can keep using DAWGit meanwhile.</p>
    {:else}
      <p class="muted">Reading the project…</p>
    {/if}
    {#if progress}<ProgressBar p={progress} />{/if}
  </div>
{:else}
  <div class="view">
    <header>
      <div class="title">
        <h1>{st.name}</h1>
        <div class="sub">
          <div class="branch-wrap">
            <button class="branch" onclick={() => (branchMenu = !branchMenu)} disabled={!st.remoteUrl}
              title={st.remoteUrl ? "Branches" : "Share the project with a team to use branches"}>
              ⑂ {st.branch} ▾
            </button>
            {#if branchMenu}
              <div class="menu" role="menu">
                <div class="menu-h">Switch to</div>
                {#each st.branches as b (b.name)}
                  <button class="item" disabled={b.current} onclick={() => switchTo(b.name)}>
                    <span>{b.name}</span>
                    <span class="faint">{b.current ? "current" : b.latest ? `${b.latest.author} · ${ago(b.latest.time)}` : ""}</span>
                  </button>
                {/each}
                <div class="sep"></div>
                <div class="menu-h">Merge into {st.branch}</div>
                {#each st.branches.filter((b) => !b.current) as b (b.name)}
                  <button class="item" onclick={() => openMergePreview(b.name)}>{b.name}</button>
                {:else}
                  <div class="item faint">no other branches</div>
                {/each}
                <div class="sep"></div>
                <button class="item" onclick={() => { branchMenu = false; newBranch = ""; }}>New branch from here…</button>
              </div>
            {/if}
          </div>
          {#if st.remoteUrl}
            <span class="dot" class:on={st.online} class:checking={!st.teamChecked}></span>
            <span class="faint" title={st.online ? st.remoteUrl : st.offline}>
              {st.teamName || st.remoteUrl}{!st.teamChecked ? " · checking…" : st.online ? "" : " · not reachable"}
            </span>
          {:else if teams.length}
            <button class="ghost" onclick={openShare}>Share with a team…</button>
          {:else}
            <span class="faint">on this computer only</span>
          {/if}
          <button class="ghost rules-btn" class:bad={!!st.rules.error} onclick={() => (rulesOpen = true)}
            title="Which files DAWGit tracks in this project">{st.rules.error ? "⚠ Rules" : "Rules"}</button>
        </div>
      </div>
      <div class="actions">
        {#if st.openable.length === 1}
          <button onclick={() => api.OpenInTool(st!.root, st!.openable[0]).catch((e) => toast(errorText(e), "error"))}
            title="Open {label(st.openable[0])} in {st.tool || "its program"}">▶ Open in {isLive ? "Live" : st.tool || "app"}</button>
        {:else if st.openable.length > 1}
          <div class="open-wrap">
            <button onclick={() => (setMenu = !setMenu)} title="Open in {st.tool || "its program"}">▶ Open in {isLive ? "Live" : st.tool || "app"} ▾</button>
            {#if setMenu}
              <div class="menu right" role="menu">
                {#each st.openable as s}
                  <button class="item" onclick={() => { setMenu = false; api.OpenInTool(st!.root, s).catch((e) => toast(errorText(e), "error")); }}>{label(s)}</button>
                {/each}
              </div>
            {/if}
          </div>
        {/if}
        <button class="ghost refresh" class:spin={refreshing} onclick={refresh} title="Refresh">↻</button>
        <button class="ghost" onclick={() => api.ShowFolder(st!.root)} title="Show folder">📁</button>
      </div>
    </header>

    {#if progress}
      <div class="banner info"><ProgressBar p={progress} team={st.teamName || undefined} /></div>
    {:else if busy === "first-share"}
      <div class="banner info"><div>Sharing “{st.name}” with the team…</div></div>
    {/if}

    {#if st.olderVersion}
      {@const v = st.olderVersion}
      <div class="banner older">
        <div>
          You're on an older version: <strong>“{v.message || v.short}”</strong>
          <span class="muted">— {v.author}, {ago(v.time)}. Newer versions are kept.</span>
        </div>
        {#if st.remoteUrl && st.changes.length}
          <button onclick={() => putOnBranch(message || "")} disabled={!!busy}
            title="Commit your changes on a branch of your own, starting from this version">New branch from here…</button>
          <button onclick={() => goTo(null)} disabled={!!busy}>Back to latest</button>
          <button class="primary" onclick={() => openCombine(message)} disabled={!!busy}
            title="Commit your changes after this version and combine them with the latest">Preview & combine</button>
        {:else if st.remoteUrl}
          <button onclick={() => (newBranch = "")} disabled={!!busy}
            title="Continue from this version on a branch of your own">New branch from here…</button>
        {:else}
          <button onclick={() => (keepOpen = `Back to “${v.message || v.short}”`)} disabled={!!busy}
            title="Continue from this version: it becomes a new, latest version">Make this the latest…</button>
        {/if}
        {#if !(st.remoteUrl && st.changes.length)}
          <button class="primary" onclick={() => goTo(null)} disabled={!!busy}>Back to latest</button>
        {/if}
      </div>
    {/if}
    {#if st.incoming.length}
      <div class="banner info">
        <div>
          <strong>{[...new Set(st.incoming.map((v) => v.author))].join(", ")}</strong>
          {st.changes.length && !st.olderVersion ? "committed" : "saved"} {st.incoming.length} new version{st.incoming.length === 1 ? "" : "s"}{st.changes.length && !st.olderVersion ? " while you were working" : ""}:
          <span class="muted">{st.incoming.slice(0, 3).map((v) => `“${v.message}”`).join(", ")}{st.incoming.length > 3 ? "…" : ""}</span>
        </div>
        {#if st.olderVersion}
          <!-- back to the latest version first -->
        {:else if st.changes.length}
          <button class="ghost" onclick={() => (discardOpen = true)} disabled={!!busy}
            title="Drop your uncommitted changes and take the team's versions">Discard my changes…</button>
          <button onclick={() => putOnBranch(message || "")} disabled={!!busy}
            title="Commit your work on a new branch; this branch stays as the team left it">Put my work on a new branch…</button>
          <button class="primary" onclick={() => openCombine(message)} disabled={!!busy}
            title="See what they changed, then combine it with your work">Preview & combine</button>
        {:else}
          <button onclick={openUpdatePreview} disabled={!!busy}>Preview</button>
          <button class="primary" onclick={() => run(updateAction)} disabled={!!busy}>Get updates</button>
        {/if}
      </div>
    {/if}

    {#if st.rules.error}
      <div class="banner warn">
        <div>⚠ {st.rules.error}</div>
        <button onclick={openRules}>Open {".dawgit.yaml"}</button>
      </div>
    {/if}

    <nav>
      <button class:on={tab === "changes"} onclick={() => (tab = "changes")}>
        Changes {#if st.changes.length}<span class="count">{st.changes.length}</span>{/if}
      </button>
      <button class:on={tab === "history"} onclick={() => (tab = "history")}>History</button>
    </nav>

    <main class:flush={tab === "changes"}>
      {#if tab === "changes"}
        {#snippet summary()}
          <section>
            {#if st!.myEdits.length}
              <h3>Tracks you changed</h3>
              <ul class="tracks">
                {#each st!.myEdits as e}
                  <li>
                    <span class="chg {e.change}"></span>
                    <span>{e.name}</span>
                    <span class="faint">{e.set}</span>
                  </li>
                {/each}
              </ul>
            {:else}
              <p class="muted">{st!.changes.length ? "Pick a file on the left to see what changed." : `No uncommitted changes. Work in ${isLive ? "Live" : st!.tool || "your app"} and save (Ctrl+S) — your changes show up here.`}</p>
            {/if}
            {#if st!.myEdits.length}<p class="faint small">Pick a file on the left for its details, history and, for samples, to listen.</p>{/if}
          </section>
        {/snippet}
        <ChangesPanel {root} st={st} {summary} ondiscard={(p) => (discardFile = p)}
          ondiscardall={() => (discardAllOpen = true)}
          onrestore={(path, version, label) => (restoreFile = { path, version, label })} />
      {:else if tab === "history"}
        <History {root} versions={st.history} head={st.head} incoming={incomingIds} latest={st.latest}
          ongoto={(v) => goTo(v)} onexport={exportVersion}
          onmerge={st.remoteUrl && !st.olderVersion ? openVersionMerge : undefined} />
      {/if}
    </main>

    {#if tab === "changes"}
      <footer class="save">
        <textarea rows="2" bind:value={message} placeholder="What did you change? e.g. “New bassline in the chorus”"
          onkeydown={(e) => { if (e.key === "Enter" && e.ctrlKey) commit(); }}></textarea>
        <div class="save-row">
          <p class="faint small">
            {#if st.olderVersion}
              {#if st.remoteUrl}
                You're on an older version. Committing combines your changes with the latest version of
                “{st.branch}” (you'll see a preview first) — or start a new branch from here.
              {:else}
                You're on an older version. Make it the latest version to commit changes, or go back to the
                latest version.
              {/if}
            {:else if st.remoteUrl}
              Commits the project folder and shares it with the team on “{st.branch}”. If others committed in
              the meantime, you'll see what they changed and choose how to combine first.
            {:else}
              Commits on this computer. Share the project with a team to work on it together.
            {/if}
          </p>
          <button class="primary" disabled={!message.trim() || !!busy || (!!st.olderVersion && !st.remoteUrl)} onclick={() => commit()}
            title="Ctrl+Enter">
            {busy === "save" || busy === "first-share" ? "Committing…" : st.remoteUrl ? "Commit version & share" : "Commit version"}
          </button>
        </div>
      </footer>
    {/if}
  </div>

  {#if combine}
    <CombineDialog preview={combine.data} branch={st.branch} older={!!st.olderVersion} bind:message={combine.message} busy={!!busy}
      onclose={() => (combine = null)} oncombine={combineAndShare} onbranch={() => putOnBranch(combine!.message)} />
  {/if}

  {#if discardAllOpen}
    <Modal title="Discard all your changes?" onclose={() => (discardAllOpen = false)}>
      <p>All {st.changes.length} uncommitted change{st.changes.length === 1 ? "" : "s"} will be lost: the project folder
        goes back to the version you're on. This can't be undone.</p>
      {#snippet footer()}
        <button onclick={() => (discardAllOpen = false)}>Cancel</button>
        <button class="danger" onclick={discardEverything}>Discard all</button>
      {/snippet}
    </Modal>
  {/if}

  {#if restoreFile}
    {@const r = restoreFile}
    {@const pending = st.changes.some((c) => c.path === r.path)}
    <Modal title="Restore {r.path.slice(r.path.lastIndexOf('/') + 1)} from “{r.label}”?" onclose={() => (restoreFile = null)}>
      <p>The file goes back to how it was in that version. The rest of the project stays as it is; commit when you're
        happy with it.</p>
      {#if pending}<p class="warn-text">This file has uncommitted changes — they'll be replaced.</p>{/if}
      {#snippet footer()}
        <button onclick={() => (restoreFile = null)}>Cancel</button>
        <button class="primary" onclick={restoreOneFile}>Restore</button>
      {/snippet}
    </Modal>
  {/if}

  {#if discardFile}
    <Modal title="Discard your changes to {discardFile.slice(discardFile.lastIndexOf('/') + 1)}?" onclose={() => (discardFile = "")}>
      <p>The file goes back to how it is in the version you're on. This can't be undone.</p>
      {#snippet footer()}
        <button onclick={() => (discardFile = "")}>Cancel</button>
        <button class="danger" onclick={discardOneFile}>Discard changes</button>
      {/snippet}
    </Modal>
  {/if}

  {#if discardOpen}
    <Modal title="Discard your changes?" onclose={() => (discardOpen = false)}>
      <p>Your {st.changes.length} uncommitted change{st.changes.length === 1 ? "" : "s"} will be lost, and the project
        gets the team's latest versions of “{st.branch}”.</p>
      <p class="muted">To keep them instead, combine them with the team's work or put them on a new branch.</p>
      {#snippet footer()}
        <button onclick={() => (discardOpen = false)}>Cancel</button>
        <button class="danger" onclick={discardAndUpdate}>Discard and update</button>
      {/snippet}
    </Modal>
  {/if}

  {#if untrackedConfirm}
    {@const files = untrackedConfirm}
    <Modal title={commitWarnings.length ? "Before you commit" : "No longer tracked"} onclose={() => (untrackedConfirm = null)}>
      {#if commitWarnings.length}
        <ul class="warnings">
          {#each commitWarnings.slice(0, 12) as w}<li>⚠ {w}</li>{/each}
          {#if commitWarnings.length > 12}<li class="faint">… and {commitWarnings.length - 12} more</li>{/if}
        </ul>
      {/if}
      {#if files.length}
        <p>The project's rules now leave {files.length === 1 ? "this file" : `these ${files.length} files`} out of versions.
          {files.length === 1 ? "It stays" : "They stay"} on this computer, and on your teammates' computers too.</p>
        <ul class="untracked mono">
          {#each files.slice(0, 12) as f}<li>{f}</li>{/each}
          {#if files.length > 12}<li class="faint">… and {files.length - 12} more</li>{/if}
        </ul>
      {/if}
      {#snippet footer()}
        <button onclick={() => (untrackedConfirm = null)}>Cancel</button>
        <button class="primary" onclick={() => { untrackedConfirm = null; commit(true); }}>
          {commitWarnings.length ? "Commit anyway" : "Commit"}</button>
      {/snippet}
    </Modal>
  {/if}

  {#if rulesOpen}
    <Modal title="Rules of “{st.name}”" onclose={() => (rulesOpen = false)}>
      <p class="muted">Which files DAWGit tracks, set in the project's <span class="mono">.dawgit.yaml</span>. The file is
        committed with the project, so the whole team uses the same rules.</p>
      <ul class="applied">
        {#each st.rules.applied as a}
          <li><strong>{a.preset === "none" ? "No preset" : a.preset === "ableton" ? "Ableton Live project"
            : st.tool ? `${st.tool} project (${a.preset})` : a.preset}</strong>
            <span class="faint">{a.folder ? `in ${a.folder}/` : "the project folder"}{a.detected ? " · detected" : ""}</span></li>
        {:else}
          <li class="faint">No preset: every file is tracked.</li>
        {/each}
      </ul>
      <p class="faint small">{st.rules.fromFile ? "Your .dawgit.yaml may add rules on top, e.g. to leave a folder out."
        : "No .dawgit.yaml yet: DAWGit follows the preset as is. Create one to leave folders out or track more."}</p>
      {#if st.rules.error}<p class="error">{st.rules.error}</p>{/if}
      {#snippet footer()}
        <button class="ghost" onclick={() => api.OpenURL("https://github.com/n0neye/DAWGit/blob/main/docs/profiles.md")}>Guide ↗</button>
        <span class="spacer"></span>
        <button onclick={() => (rulesOpen = false)}>Close</button>
        <button class="primary" onclick={() => { rulesOpen = false; openRules(); }}>{st?.rules.fromFile ? "Open .dawgit.yaml" : "Create .dawgit.yaml"}</button>
      {/snippet}
    </Modal>
  {/if}

  {#if preview}
    {@const p = preview}
    <PreviewDialog title={p.title} preview={p.data} actionLabel={p.label} blocked={p.blocked} bind:message={mergeMessage}
      onclose={() => (preview = null)}
      onconfirm={() => { const action = p.run; preview = null; run(action); }} />
  {/if}

  {#if conflicts}
    {@const c = conflicts}
    <ConflictDialog conflicts={c.items} onclose={() => (conflicts = null)}
      onresolve={(res) => { const { run: action, force } = c; conflicts = null; run(action, res, force); }} />
  {/if}

  {#if liveBlocked}
    {@const b = liveBlocked}
    <Modal title={isLive || !st?.tool ? (b.set ? `“${b.set}” is open in Live` : "Ableton Live is running")
      : `${st.tool} has this project open`} onclose={() => (liveBlocked = null)}>
      {#if isLive || !st?.tool}
        <p>DAWGit is about to change files in this project. Live keeps the open set in memory and would
          overwrite the changes the next time you save it.</p>
        <p class="muted">Save and close the set in Live first — you can leave Live open with another set.
          {#if b.set}Live only shows the set's name, so a set with the same name from another project counts too.
          {:else}DAWGit cannot tell which set Live has open: close Live to go on.{/if}</p>
      {:else}
        <p>DAWGit is about to change files in this project. {st.tool} may hold some of them open, or write over
          the changes.</p>
        <p class="muted">Save your work and close the project in {st.tool} first.</p>
      {/if}
      {#snippet footer()}
        <button onclick={() => (liveBlocked = null)}>Cancel</button>
        <button class="primary" onclick={() => { const { run: action, resolutions } = b; liveBlocked = null; run(action, resolutions); }}>
          I closed it — continue
        </button>
      {/snippet}
    </Modal>
  {/if}

  {#if shareOpen}
    <Modal title="Share “{st.name}” with a team" onclose={() => (shareOpen = false)}>
      <p class="muted">DAWGit commits a first version and uploads it, including its samples, so your teammates
        can download it.</p>
      <div class="teams">
        {#each teams as t (t.id)}
          <label class="team"><input type="radio" bind:group={shareTeam} value={t.id} /> {t.name}
            <span class="faint">{t.address}</span></label>
        {/each}
      </div>
      {#snippet footer()}
        <button onclick={() => (shareOpen = false)}>Cancel</button>
        <button class="primary" disabled={!shareTeam || !!busy} onclick={shareWithTeam}>Share</button>
      {/snippet}
    </Modal>
  {/if}

  {#if leaving}
    {@const l = leaving}
    <Modal title={l.target ? "Go to an older version" : "Back to the latest version"} onclose={() => (leaving = null)}>
      <p>You have {st.changes.length} uncommitted change{st.changes.length === 1 ? "" : "s"}.
        {l.target ? "Going to another version" : "Going back"} replaces the files in the project folder.</p>
      {#if !st.olderVersion}
        <label for="lm">Commit them first as</label>
        <input id="lm" bind:value={l.message} placeholder="What did you change?" />
      {:else}
        <p class="muted">Changes made on an older version can be kept by starting a new branch from here
          {st.remoteUrl ? "" : "or making it the latest version"} first.</p>
      {/if}
      {#snippet footer()}
        <button onclick={() => (leaving = null)}>Cancel</button>
        <button class="danger" onclick={discardThenGo}>Discard changes</button>
        {#if !st!.olderVersion}
          <button class="primary" disabled={!l.message.trim()} onclick={commitThenGo}>
            {st!.remoteUrl ? "Commit & share, then go" : "Commit, then go"}
          </button>
        {/if}
      {/snippet}
    </Modal>
  {/if}

  {#if keepOpen !== null}
    <Modal title="Make this the latest version" onclose={() => (keepOpen = null)}>
      <p class="muted">The older version you are on (with any changes you made) becomes a new version on top of the
        latest one. Nothing in the history is lost.</p>
      <label for="km">Describe it</label>
      <input id="km" bind:value={keepOpen} />
      {#snippet footer()}
        <button onclick={() => (keepOpen = null)}>Cancel</button>
        <button class="primary" disabled={!keepOpen?.trim() || !!busy} onclick={keepThisVersion}>Make it the latest</button>
      {/snippet}
    </Modal>
  {/if}

  {#if newBranch !== null}
    <Modal title="New branch" onclose={() => (newBranch = null)}>
      <p class="muted">A branch is your own line of versions (e.g. to try an idea). The team keeps working on
        “{st.branch}”; merge back when you're happy.</p>
      <label for="bn">Branch name</label>
      <input id="bn" bind:value={newBranch} placeholder="yi-chorus-idea" />
      {#snippet footer()}
        <button onclick={() => (newBranch = null)}>Cancel</button>
        <button class="primary" disabled={!newBranch?.trim() || busy === "branch"} onclick={createBranch}>Create</button>
      {/snippet}
    </Modal>
  {/if}
{/if}

<style>
  .view { display: flex; flex-direction: column; height: 100%; }
  .teams { display: flex; flex-direction: column; gap: 6px; }
  .team { display: flex; align-items: center; gap: 8px; margin: 0; color: var(--text); font-size: 14px; }
  .team input { width: auto; }
  .pad { padding: 24px; }
  /* Same place as the loaded header's title, so nothing jumps. */
  .preparing { padding: 18px 24px; max-width: 600px; }
  .refresh.spin { animation: spin .8s linear infinite; }
  @keyframes spin { to { transform: rotate(360deg); } }
  .error { color: var(--danger); }
  header { display: flex; align-items: flex-start; padding: 18px 24px 10px; gap: 16px; }
  .title { flex: 1; min-width: 0; }
  h1 { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  h1 { margin: 0 0 6px; font-size: 22px; font-weight: 650; }
  .sub { display: flex; align-items: center; gap: 10px; }
  .actions { display: flex; gap: 8px; flex: none; }
  .open-wrap { position: relative; }
  .menu.right { left: auto; right: 0; min-width: 220px; max-height: 50vh; overflow: auto; }
  .branch { padding: 3px 10px; font-size: 13px; }
  .branch-wrap { position: relative; }
  .menu {
    position: absolute; top: 32px; left: 0; z-index: 20; min-width: 260px; padding: 6px;
    background: var(--panel-2); border: 1px solid var(--line); border-radius: 8px;
    box-shadow: 0 12px 30px rgba(0, 0, 0, .45);
  }
  .menu-h { font-size: 11px; text-transform: uppercase; letter-spacing: .06em; color: var(--faint); padding: 6px 8px 2px; }
  .item { display: flex; justify-content: space-between; width: 100%; border: none; background: transparent; padding: 6px 8px; text-align: left; gap: 12px; }
  .item:hover:not(:disabled) { background: #33363d; }
  .sep { height: 1px; background: var(--line); margin: 6px 0; }
  .dot { width: 8px; height: 8px; border-radius: 50%; background: var(--danger); }
  .dot.on { background: var(--accent); }
  .dot.checking { background: var(--faint); }
  .rules-btn { padding: 2px 8px; font-size: 12.5px; color: var(--faint); }
  .rules-btn:hover { color: var(--text); }
  .rules-btn.bad { color: var(--warn); }
  .untracked { list-style: none; padding: 8px 12px; margin: 10px 0 0; background: var(--bg); border-radius: 8px;
    font-size: 12.5px; max-height: 220px; overflow: auto; }
  .warnings { list-style: none; padding: 0; margin: 0 0 12px; display: flex; flex-direction: column; gap: 6px;
    color: var(--warn); font-size: 13.5px; user-select: text; }
  .applied { list-style: none; padding: 0; margin: 12px 0; display: flex; flex-direction: column; gap: 6px; }
  .applied li { display: flex; gap: 10px; align-items: baseline; }

  .banner { display: flex; align-items: center; gap: 10px; margin: 6px 24px; padding: 10px 14px; border-radius: 8px; }
  .banner > div { flex: 1; }
  .banner.info { background: #1d2c38; border: 1px solid #2c4557; }
  .banner.older { background: #2a2536; border: 1px solid #463c5c; }
  .banner.warn { background: var(--warn-bg); border: 1px solid #5a4623; color: #f0d9a8; }

  nav { display: flex; gap: 4px; padding: 10px 24px 0; border-bottom: 1px solid var(--line); }
  nav button { border: none; background: transparent; border-radius: 6px 6px 0 0; padding: 8px 14px; color: var(--muted); border-bottom: 2px solid transparent; }
  nav button.on { color: var(--text); border-bottom-color: var(--accent); }
  .count { margin-left: 4px; font-size: 11px; padding: 0 6px; border-radius: 8px; background: #33363d; }

  main { flex: 1; overflow: auto; padding: 16px 24px 32px; }
  .warn-text { color: var(--warn); }
  main.flush { padding: 0 0 0 16px; overflow: hidden; min-height: 0; }
  h3 { font-size: 12px; text-transform: uppercase; letter-spacing: .06em; color: var(--muted); margin: 6px 0 10px; }
  .save { border-top: 1px solid var(--line); background: var(--panel); padding: 12px 24px 14px; }
  .save textarea { width: 100%; resize: vertical; min-height: 44px; }
  .save-row { display: flex; align-items: center; gap: 16px; margin-top: 8px; }
  .save-row p { flex: 1; margin: 0; }
  .save-row button { padding: 9px 18px; white-space: nowrap; }
  .small { font-size: 12px; margin: 10px 0 0; }

  .tracks { list-style: none; padding: 0; margin: 0 0 18px; display: flex; flex-direction: column; gap: 4px; }
  .tracks li { display: flex; align-items: center; gap: 10px; }
  .chg { width: 8px; height: 8px; border-radius: 2px; background: var(--mod); }
  .chg.added { background: var(--add); }
  .chg.removed { background: var(--del); }
</style>
