<script lang="ts">
  import { t, tn } from "./i18n.svelte";
  import Tx from "./Tx.svelte";
  import { untrack } from "svelte";
  import { Events } from "@wailsio/runtime";
  import { api, ago, errorText, formatBytes, type State, type Result, type Preview, type Conflict, type TeamSummary,
    type Progress, type Version, type RuleSuggestion } from "./api";
  import { toast } from "./notify.svelte";
  import { cachedState, rememberState } from "./stateCache";
  import ChangesPanel from "./ChangesPanel.svelte";
  import CombineDialog from "./CombineDialog.svelte";
  import ProgressBar from "./ProgressBar.svelte";
  import History from "./History.svelte";
  import Modal from "./Modal.svelte";
  import PreviewDialog from "./PreviewDialog.svelte";
  import ConflictDialog from "./ConflictDialog.svelte";

  // firstShare: the project was just added (to a team, or this computer).
  // With no versions yet, it asks whether to commit (and share) a first one
  // now or after a look through the files; a project with versions shares
  // them right away.
  let { root, refreshKey, teams, firstShare = false, onchanged, onfirstshared, onsettings }: {
    root: string; refreshKey: number; teams: TeamSummary[]; firstShare?: boolean;
    onchanged: () => void; onfirstshared?: () => void;
    onsettings: () => void; // the project's settings (name, rules, …)
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
  const UNSAVED = () => t("You have uncommitted changes. Commit a version instead — the team's changes are merged in as part of it, and anything you both changed is shown then.");
  let conflicts = $state<{ items: Conflict[]; run: Action; force: boolean } | null>(null);
  let liveBlocked = $state<{ run: Action; resolutions: Record<string, string>; set: string } | null>(null);
  let branchMenu = $state(false);
  let setMenu = $state(false);
  let newBranch = $state<string | null>(null);
  // Go to version: asked first when there are uncommitted changes.
  let leaving = $state<{ target: Version | null; message: string } | null>(null); // null target: latest
  let keepOpen = $state<string | null>(null); // message for "Make this the latest version"

  // message: for a save, what to describe the version with (asked again when
  // the team committed in the meantime: see combine).
  // The warning about OneDrive & co., once understood, stays away (per project).
  let cloudOk = $state<Record<string, boolean>>({});
  $effect.pre(() => {
    try { cloudOk[root] = localStorage.getItem(`dawgit.cloudOk:${root}`) === "1"; } catch { /* shown */ }
  });
  function okCloud(r: string) {
    cloudOk[r] = true;
    try { localStorage.setItem(`dawgit.cloudOk:${r}`, "1"); } catch { /* shown again next time */ }
  }

  type Action = { name: string; call: (res: Record<string, string>, force: boolean) => Promise<Result | null>;
    done: (r: Result) => void; message?: string };

  // Teammates committed on this branch while you were working: preview, then
  // combine, put your work on a branch, or discard it.
  let combine = $state<{ data: Preview; message: string } | null>(null);
  let branchThenCommit = $state(""); // commit this after creating the branch
  let discardOpen = $state(false);
  let discardFile = $state(""); // one file's changes, after confirming
  let discardAllOpen = $state(false);
  let discardSome = $state<string[] | null>(null); // the ticked changes, after confirming
  let restoreFile = $state<{ path: string; version: string; label: string; source: string } | null>(null);

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

  $effect(() => {
    root; refreshKey;
    load();
  });

  // Just added: ask about the first version once the project is read.
  let firstAsk = $state(false);
  let shareAsk = $state(false);
  let firstShareStarted = false;
  $effect(() => {
    if (!st || firstShareStarted || !untrack(() => firstShare)) return;
    firstShareStarted = true;
    onfirstshared?.();
    if (!st.head) askFirstVersion();
    else if (st.remoteUrl) shareAsk = true; // versions already: share them now or later
  });
  function askFirstVersion() {
    if (!message.trim()) message = t("First version");
    firstAsk = true;
  }

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
  // The team's new versions to tell about: a merge only combines the others.
  let news = $derived.by(() => {
    const own = st?.incoming.filter((v) => v.parents.length < 2) ?? [];
    return own.length ? own : (st?.incoming ?? []);
  });

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
        if (r.relinked.length) toast(tn(r.relinked.length, "Relinked {n} sample path for this computer", "Relinked {n} sample paths for this computer"), "info");
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
        "published": t("Version committed and shared with the team"),
        "saved-locally": t("Version committed on this computer (not shared with a team)"),
        "fast-forward": t("You had nothing new; updated to the team's latest version"),
        "nothing": t("Nothing changed since your last version"),
      };
      toast(text[r.action] ?? t("Version committed"), r.action === "nothing" ? "info" : "ok");
      if (r.log.length && r.action === "published") toast(t("The team's changes were merged into your files") + reopen(), "warn", 9000);
      if (r.action !== "nothing") message = "";
  };

  // Changes left out of the next commit (unticked in the Changes list),
  // per project; a commit starts over with all ticked.
  let excluded = $state<Record<string, boolean>>({});
  $effect.pre(() => { root; excluded = {}; });
  let leftOut = $derived(st?.changes.filter((c) => excluded[c.path]).length ?? 0);
  // The commit shortcut as the keyboard says it.
  const isMac = /Mac|iPhone|iPad/.test(navigator.platform || navigator.userAgent);
  // The changes to commit, or none for all of them.
  // (a move commits both its places)
  const picked = () => (leftOut && st ? st.changes.filter((c) => !excluded[c.path]).flatMap((c) => (c.from ? [c.path, c.from] : [c.path])) : []);

  const saveAction = (msg: string, combineWithTeam = false): Action => {
    const paths = picked();
    return {
      name: "save",
      message: msg,
      call: (res, force) => api.Save(root, msg, combineWithTeam, res, force, paths),
      done: (r) => { excluded = {}; saveDone(r); },
    };
  };

  // Commit from the commit box: when the team is ahead, ask first.
  async function commit(confirmed = false, rulesAsked = false) {
    if (!message.trim() || busy || (st?.olderVersion && !st.remoteUrl)) return;
    if (st && st.changes.length && leftOut === st.changes.length) return; // nothing ticked
    if (!confirmed && !rulesAsked && st?.rules.suggestions.length) {
      rulesAsk = true;
      return;
    }
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
  const reopen = () => (st?.tool === "Ableton Live" ? " — " + t("reopen the set in Live")
    : st?.tool ? " — " + t("switch back to {tool} to load the changes", { tool: t(st.tool) }) : "");
  const label = (rel: string) => (rel === "." ? st?.name ?? t("the project") : rel);

  // Projects of tools found in folders the rules don't name yet: their
  // preset is suggested, and asked about before committing (or their
  // caches would go up with the version).
  const presetName = (p: string) => ({ ableton: "Ableton Live", unity: "Unity", unreal: "Unreal",
    design: t("design"), code: t("code") } as Record<string, string>)[p] ?? p;
  // What a preset leaves out, for people: "Library, Temp, Obj and 9 more".
  const leftOutText = (pats: string[]) => {
    const names = [...new Set(pats.map((p) => p.replace(/^\/|\/$/g, "")))];
    return names.length > 4 ? t("{names} and {n} more", { names: names.slice(0, 4).join(", "), n: names.length - 4 }) : names.join(", ");
  };
  let rulesAsk = $state(false);
  async function setPreset(s: RuleSuggestion, preset: string) {
    try {
      await api.SetPreset(root, s.folder, preset);
      await refresh();
      if (rulesAsk && !st?.rules.suggestions.length) {
        rulesAsk = false;
        commit(false, true);
      }
    } catch (e) {
      toast(errorText(e), "error");
    }
  }

  // The project's rules (.dawgit.yaml): files no longer tracked, the dialog.
  let untrackedConfirm = $state<string[] | null>(null);
  let commitWarnings = $state<string[]>([]);
  async function openRules() {
    try {
      await api.OpenRules(root);
      toast(t("Save the file, then DAWGit follows the new rules"), "info");
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
      done: () => toast(t("Discarded all your uncommitted changes") + reopen(), "ok"),
    });
  }

  function discardTicked() {
    const paths = discardSome!;
    discardSome = null;
    run({
      name: "discard",
      call: (_res, force) => api.DiscardFiles(root, paths, force),
      done: () => toast(tn(paths.length, "Discarded your changes to {n} file", "Discarded your changes to {n} files") + reopen(), "ok"),
    });
  }

  function restoreOneFile() {
    const r = restoreFile!;
    restoreFile = null;
    run({
      name: "restore",
      call: (_res, force) => api.RestoreFileVersion(root, r.path, r.version, r.source, force),
      done: () => toast(t("Restored {file} from “{version}” — commit it to keep it", { file: r.path.slice(r.path.lastIndexOf("/") + 1), version: r.label }), "ok", 8000),
    });
  }

  function discardOneFile() {
    const path = discardFile;
    discardFile = "";
    run({
      name: "discard",
      call: (_res, force) => api.DiscardFile(root, path, st?.changes.find((c) => c.path === path)?.from ?? "", force),
      done: () => toast(t("Discarded your changes to {file}", { file: path.slice(path.lastIndexOf("/") + 1) }), "ok"),
    });
  }

  function discardAndUpdate() {
    discardOpen = false;
    run({
      name: "update",
      call: (res, force) => api.DiscardAndUpdate(root, res, force),
      done: () => toast(t("Your changes were discarded and you have the team's latest versions") + reopen(), "ok", 8000),
    });
  }

  const updateAction: Action = {
    name: "update",
    call: (res, force) => api.Update(root, res, force),
    done: (r) => {
      if (r.action === "fast-forward" || r.action === "merged") {
        toast(t("You're up to date") + reopen(), "ok", 8000);
        if (r.action === "merged") toast(t("Your versions and the team's were combined. Commit a version to share the result."), "info", 9000);
      } else toast(t("Already up to date"), "info");
    },
  };

  async function openUpdatePreview() {
    busy = "preview";
    try {
      const data = await api.PreviewUpdate(root);
      mergeMessage = null;
      if (data) preview = { title: t("Updates from the team"), label: t("Get updates"), data, run: updateAction,
        blocked: st?.changes.length ? UNSAVED() : "" };
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
        title: t("Merge “{from}” into “{into}”", { from: name, into: st?.branch ?? "" }), label: t("Merge and share"), data,
        blocked: st?.changes.length ? t("You have uncommitted changes. Commit a version first, then merge.") : "",
        run: {
          name: "merge",
          call: (res, force) => api.MergeBranch(root, name, mergeMessage ?? "", res, force),
          done: (r) => toast(r.action === "up-to-date" || r.action === "ahead"
            ? t("Nothing to merge from {from}", { from: name }) : t("Merged {from} into {into} and shared it", { from: name, into: st?.branch ?? "" }) + reopen(), "ok", 8000),
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
        title: t("Merge {from} into “{into}”", { from: label, into: st?.branch ?? "" }), label: t("Merge and share"), data,
        blocked: st?.changes.length ? t("You have uncommitted changes. Commit a version first, then merge.") : "",
        run: {
          name: "merge",
          call: (res, force) => api.MergeVersion(root, v.id, mergeMessage ?? "", res, force),
          done: (r) => toast(r.action === "up-to-date" || r.action === "ahead"
            ? t("“{into}” already has {from}", { into: st?.branch ?? "", from: label }) : t("Merged {from} into {into} and shared it", { from: label, into: st?.branch ?? "" }) + reopen(), "ok", 8000),
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
      done: () => toast(t("Now working on “{branch}”", { branch: name }) + reopen(), "ok", 8000),
    });
  }

  // --- versions: go to, back to latest, keep, export ---

  const goAction = (id: string, discard: boolean, label: string): Action => ({
    name: "goto",
    call: (_res, force) => api.GoToVersion(root, id, discard, force),
    done: () => toast((id === "latest" ? t("Back to the latest version") : t("Now on “{version}”", { version: label })) + reopen(), "ok", 8000),
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
      call: (res, force) => api.Save(root, l.message, false, res, force, []),
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
      done: () => toast(t("This version is now the latest"), "ok"),
    });
  }

  async function exportVersion(v: Version) {
    const parent = await api.ChooseFolder(t("Where should the copy of this version go?"));
    if (!parent) return;
    busy = "export";
    try {
      const dir = await api.ExportVersion(root, v.id, parent);
      toast(t("Saved a copy of “{version}” as {folder}", { version: v.message || v.short, folder: dir }), "ok", 9000);
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
          toast(t("Your work is on the new branch “{branch}”; “{base}” is unchanged. Merge it when you're ready.", { branch: name, base: st?.branch ?? "" }), "info", 9000); } });
        return;
      }
      toast(t("Created “{branch}”. Versions you save now go there.", { branch: name }), "ok");
      await load();
      onchanged();
    } catch (e) {
      toast(errorText(e), "error");
    } finally {
      busy = "";
    }
  }

  // Shares the versions of a project that joined a team (the files aren't
  // committed: what isn't yet stays in Changes).
  function shareVersions() {
    shareAsk = false;
    run({
      name: "first-share",
      call: () => api.ShareVersions(root),
      done: () => {
        toast(t("“{name}” is shared with {team}", { name: st?.name ?? folderName, team: st?.teamName || t("the team") }), "ok");
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
      <p class="muted">{isLive ? t("Sharing with the team: DAWGit commits a first version and uploads it, samples included. Large projects can take a few minutes — you can keep using DAWGit meanwhile.")
        : t("Sharing with the team: DAWGit commits a first version and uploads it. Large projects can take a few minutes — you can keep using DAWGit meanwhile.")}</p>
    {:else}
      <p class="muted">{t("Reading the project…")}</p>
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
              title={st.remoteUrl ? t("Branches") : t("Share the project with a team to use branches")}>
              ⑂ {st.branch} ▾
            </button>
            {#if branchMenu}
              <div class="menu" role="menu">
                <div class="menu-h">{t("Switch to")}</div>
                {#each st.branches as b (b.name)}
                  <button class="item" disabled={b.current} onclick={() => switchTo(b.name)}>
                    <span>{b.name}</span>
                    <span class="faint">{b.current ? t("current") : b.latest ? `${b.latest.author} · ${ago(b.latest.time)}` : ""}</span>
                  </button>
                {/each}
                <div class="sep"></div>
                <div class="menu-h">{t("Merge into {branch}", { branch: st.branch })}</div>
                {#each st.branches.filter((b) => !b.current) as b (b.name)}
                  <button class="item" onclick={() => openMergePreview(b.name)}>{b.name}</button>
                {:else}
                  <div class="item faint">{t("no other branches")}</div>
                {/each}
                <div class="sep"></div>
                <button class="item" onclick={() => { branchMenu = false; newBranch = ""; }}>{t("New branch from here…")}</button>
              </div>
            {/if}
          </div>
          {#if st.remoteUrl}
            <span class="dot" class:on={st.online} class:checking={!st.teamChecked}></span>
            <span class="faint" title={st.online ? st.remoteUrl : st.offline}>
              {st.teamName || st.remoteUrl}{!st.teamChecked ? ` · ${t("checking…")}` : st.online ? "" : ` · ${t("not reachable")}`}
            </span>
          {/if}
          <button class="ghost gear" class:bad={!!st.rules.error} onclick={onsettings}
            title={st.rules.error ? `${t("Project settings")} — ⚠ ${st.rules.error}` : t("Project settings: name, rules, …")}
            aria-label={t("Project settings")}>{st.rules.error ? "⚠" : ""}<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z"/><circle cx="12" cy="12" r="3"/></svg></button>
        </div>
      </div>
      <div class="actions">
        {#if st.openable.length === 1}
          <button onclick={() => api.OpenInTool(st!.root, st!.openable[0]).catch((e) => toast(errorText(e), "error"))}
            title={t("Open {file} in {tool}", { file: label(st.openable[0]), tool: (st.tool ? t(st.tool) : t("its program")) })}>▶ {t("Open in {tool}", { tool: isLive ? "Live" : st.tool ? t(st.tool) : t("app") })}</button>
        {:else if st.openable.length > 1}
          <div class="open-wrap">
            <button onclick={() => (setMenu = !setMenu)} title={t("Open in {tool}", { tool: (st.tool ? t(st.tool) : t("its program")) })}>▶ {t("Open in {tool}", { tool: isLive ? "Live" : st.tool ? t(st.tool) : t("app") })} ▾</button>
            {#if setMenu}
              <div class="menu right" role="menu">
                {#each st.openable as s}
                  <button class="item" onclick={() => { setMenu = false; api.OpenInTool(st!.root, s).catch((e) => toast(errorText(e), "error")); }}>{label(s)}</button>
                {/each}
              </div>
            {/if}
          </div>
        {/if}
        <button class="ghost refresh" class:spin={refreshing} onclick={refresh} title={t("Refresh")}>↻</button>
        <button class="ghost" onclick={() => api.ShowFolder(st!.root)} title={t("Show folder")}>📁</button>
      </div>
    </header>

    {#if progress}
      <div class="banner info"><ProgressBar p={progress} team={st.teamName || undefined} /></div>
    {:else if busy === "first-share"}
      <div class="banner info"><div>{t("Sharing “{name}” with the team…", { name: st.name })}</div></div>
    {/if}

    {#if st.remoteUrl && !st.head && !busy && !progress}
      <div class="banner info">
        <div>{t("Not shared with {team} yet. Look through the files and ignore the folders or files you don't need (right-click › Ignore), then commit a first version to share it.", { team: st.teamName || t("the team") })}</div>
      </div>
    {:else if st.remoteUrl && st.unshared && !busy && !progress}
      <div class="banner info">
        <div>{t("Not shared with {team} yet: its versions are on this computer only.", { team: st.teamName || t("the team") })}</div>
        <button class="primary" onclick={shareVersions}>{t("Share now")}</button>
      </div>
    {/if}

    {#if st.cloudFolder && !cloudOk[root]}
      <div class="banner warn">
        <div>
          <Tx text={t("This project is in your {cloud} folder.")} strong={{ cloud: st.cloudFolder }} />
          <span class="muted">{t("{cloud} also syncs DAWGit's history (the hidden .dawgit folder): used from two computers it can damage it, and files kept online-only aren't really here. Best keep projects in a folder {cloud} doesn't sync: DAWGit and your team storage already keep them safe.", { cloud: st.cloudFolder })}</span>
        </div>
        <button onclick={() => okCloud(root)}>{t("I understand")}</button>
      </div>
    {/if}
    {#if st.unfinished}
      {@const v = st.unfinished}
      <div class="banner warn">
        <div>
          <Tx text={t("Switching to {version} didn't finish")} strong={{ version: `“${v.message || v.short}”` }} />
          <span class="muted">{t("— DAWGit was closed or a file was in use. Some files are from that version, some aren't. Put them back as they were, then try again.")}</span>
        </div>
        <button class="primary" disabled={!!busy} onclick={() => run({ name: "goto", message: "",
          call: (_res, force) => api.RecoverSwitch(root, force),
          done: () => toast(t("Files put back as they were"), "ok") })}>{t("Put files back")}</button>
      </div>
    {/if}
    {#each st.rules.suggestions as s (s.folder + s.preset)}
      <div class="banner warn">
        <div>{@render suggestionText(s)}</div>
        {@render suggestionButtons(s)}
      </div>
    {/each}
    {#if st.olderVersion}
      {@const v = st.olderVersion}
      <div class="banner older">
        <div>
          <Tx text={t("You're on an older version: {version}")} strong={{ version: `“${v.message || v.short}”` }} />
          <span class="muted">— {v.author}, {ago(v.time)}. {t("Newer versions are kept.")}</span>
        </div>
        {#if st.remoteUrl && st.changes.length}
          <button onclick={() => putOnBranch(message || "")} disabled={!!busy}
            title={t("Commit your changes on a branch of your own, starting from this version")}>{t("New branch from here…")}</button>
          <button onclick={() => goTo(null)} disabled={!!busy}>{t("Back to latest")}</button>
          <button class="primary" onclick={() => openCombine(message)} disabled={!!busy}
            title={t("Commit your changes after this version and combine them with the latest")}>{t("Preview & combine")}</button>
        {:else if st.remoteUrl}
          <button onclick={() => (newBranch = "")} disabled={!!busy}
            title={t("Continue from this version on a branch of your own")}>{t("New branch from here…")}</button>
        {:else}
          <button onclick={() => (keepOpen = t("Back to “{version}”", { version: v.message || v.short }))} disabled={!!busy}
            title={t("Continue from this version: it becomes a new, latest version")}>{t("Make this the latest…")}</button>
        {/if}
        {#if !(st.remoteUrl && st.changes.length)}
          <button class="primary" onclick={() => goTo(null)} disabled={!!busy}>{t("Back to latest")}</button>
        {/if}
      </div>
    {/if}
    {#if st.incoming.length}
      <div class="banner info">
        <div>
          <Tx text={st.changes.length && !st.olderVersion
            ? tn(news.length, "{who} committed {n} new version while you were working:", "{who} committed {n} new versions while you were working:")
            : tn(news.length, "{who} saved {n} new version:", "{who} saved {n} new versions:")}
            strong={{ who: [...new Set(news.map((v) => v.author))].join(", ") }} />
          <span class="muted">{news.slice(0, 3).map((v) => `“${v.message}”`).join(", ")}{news.length > 3 ? "…" : ""}</span>
        </div>
        {#if st.olderVersion}
          <!-- back to the latest version first -->
        {:else if st.changes.length}
          <button class="ghost" onclick={() => (discardOpen = true)} disabled={!!busy}
            title={t("Drop your uncommitted changes and take the team's versions")}>{t("Discard my changes…")}</button>
          <button onclick={() => putOnBranch(message || "")} disabled={!!busy}
            title={t("Commit your work on a new branch; this branch stays as the team left it")}>{t("Put my work on a new branch…")}</button>
          <button class="primary" onclick={() => openCombine(message)} disabled={!!busy}
            title={t("See what they changed, then combine it with your work")}>{t("Preview & combine")}</button>
        {:else}
          <button onclick={openUpdatePreview} disabled={!!busy}>{t("Preview")}</button>
          <button class="primary" onclick={() => run(updateAction)} disabled={!!busy}>{t("Get updates")}</button>
        {/if}
      </div>
    {/if}

    {#if st.rules.error}
      <div class="banner warn">
        <div>⚠ {st.rules.error}</div>
        <button onclick={openRules}>{t("Open {file}", { file: ".dawgit.yaml" })}</button>
      </div>
    {/if}

    <nav>
      <button class:on={tab === "changes"} onclick={() => (tab = "changes")}>
        {t("Changes")} {#if st.changes.length}<span class="count">{st.changes.length}</span>{/if}
      </button>
      <button class:on={tab === "history"} onclick={() => (tab = "history")}>{t("History")}</button>
    </nav>

    <main class:flush={tab === "changes"}>
      {#if tab === "changes"}
        {#snippet summary()}
          <section>
            {#if st!.myEdits.length}
              <h3>{t("Tracks you changed")}</h3>
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
              <p class="muted">{st!.changes.length ? t("Pick a file on the left to see what changed.") : t("No uncommitted changes. Work in {tool} and save (Ctrl+S) — your changes show up here.", { tool: isLive ? "Live" : st!.tool ? t(st!.tool) : t("your app") })}</p>
            {/if}
            {#if st!.myEdits.length}<p class="faint small">{t("Pick a file on the left for its details, history and, for samples, to listen.")}</p>{/if}
          </section>
        {/snippet}
        {#snippet commitBox()}
          <textarea rows="3" bind:value={message} placeholder={t("What did you change? e.g. “New bassline in the chorus”")}
            onkeydown={(e) => { if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) commit(); }}></textarea>
          {#if st!.olderVersion}
            <p class="faint small older">{st!.remoteUrl
              ? t("You're on an older version. Committing combines your changes with the latest version of “{branch}” (you'll see a preview first) — or start a new branch from here.", { branch: st!.branch })
              : t("You're on an older version. Make it the latest version to commit changes, or go back to the latest version.")}</p>
          {/if}
          <button class="primary commit-btn" disabled={!message.trim() || !!busy || (!!st!.olderVersion && !st!.remoteUrl)
            || (st!.changes.length > 0 && leftOut === st!.changes.length)} onclick={() => commit()}
            title={st!.remoteUrl
              ? t("Commits the project folder and shares it with the team on “{branch}”. If others committed in the meantime, you'll see what they changed and choose how to combine first.", { branch: st!.branch }) + (leftOut ? " " + t("Unticked files stay uncommitted.") : "")
              : t("Commits on this computer. Share the project with a team to work on it together.") + (leftOut ? " " + t("Unticked files stay uncommitted.") : "")}>
            <span>{busy === "save" || busy === "first-share" ? t("Committing…")
              : leftOut ? t(st!.remoteUrl ? "Commit {done} of {total} & Share" : "Commit {done} of {total}", { done: st!.changes.length - leftOut, total: st!.changes.length })
              : st!.remoteUrl ? t("Commit & Share") : t("Commit")}</span>
            <kbd>{isMac ? "⌘" : "Ctrl"} ↵</kbd>
          </button>
        {/snippet}
        <ChangesPanel {root} st={st} {summary} {commitBox} bind:excluded onrules={() => load()} ondiscard={(p) => (discardFile = p)}
          ondiscardall={() => (discardAllOpen = true)} ondiscardsome={(paths) => (discardSome = paths)}
          onrestore={(path, version, label, source) => (restoreFile = { path, version, label, source })} />
      {:else if tab === "history"}
        <History {root} versions={st.history} head={st.head} incoming={incomingIds} latest={st.latest}
          ongoto={(v) => goTo(v)} onexport={exportVersion}
          onmerge={st.remoteUrl && !st.olderVersion ? openVersionMerge : undefined} />
      {/if}
    </main>

  </div>

  {#if shareAsk}
    <Modal title={t("Share “{name}” with {team}?", { name: st.name, team: st.teamName || t("the team") })} onclose={() => (shareAsk = false)}>
      <p>{isLive ? tn(st.history.length, "Upload its {n} version now, samples included?", "Upload its {n} versions now, samples included?")
        : tn(st.history.length, "Upload its {n} version now?", "Upload its {n} versions now?")}</p>
      <p class="muted">{t("Or later: first look through the files and ignore the folders or files you don't need (right-click › Ignore), then share from the banner at the top. Your team sees the project once it's shared.")}{st.changes.length
          ? " " + tn(st.changes.length, "The {n} uncommitted change stays in Changes either way.", "The {n} uncommitted changes stay in Changes either way.") : ""}</p>
      {#snippet footer()}
        <button onclick={() => (shareAsk = false)}>{t("Later")}</button>
        <button class="primary" onclick={shareVersions}>{t("Share now")}</button>
      {/snippet}
    </Modal>
  {/if}

  {#if firstAsk}
    <Modal title={t("“{name}” is added", { name: st.name })} onclose={() => (firstAsk = false)}>
      <p>{st.remoteUrl
        ? t(isLive ? "Commit a first version now and share it with {team}, samples included?" : "Commit a first version now and share it with {team}?", { team: st.teamName || t("the team") })
        : t("Commit a first version now?")}</p>
      <p class="muted">{tn(st.changes.length, "Or later: first look through the {n} file and ignore the folders or files you don't need (right-click › Ignore), then commit from the Changes tab.",
        "Or later: first look through the {n} files and ignore the folders or files you don't need (right-click › Ignore), then commit from the Changes tab.")}{st.remoteUrl ? " " + t("Your team sees the project once it's committed.") : ""}</p>
      <label for="first-msg">{t("Message")}</label>
      <input id="first-msg" bind:value={message} onkeydown={(e) => { if (e.key === "Enter" && message.trim()) { firstAsk = false; commit(); } }} />
      {#snippet footer()}
        <button onclick={() => (firstAsk = false)}>{t("Later")}</button>
        <button class="primary" disabled={!message.trim()} onclick={() => { firstAsk = false; commit(); }}>
          {st?.remoteUrl ? t("Commit & Share now") : t("Commit now")}</button>
      {/snippet}
    </Modal>
  {/if}

  {#if combine}
    <CombineDialog preview={combine.data} branch={st.branch} older={!!st.olderVersion} bind:message={combine.message} busy={!!busy}
      onclose={() => (combine = null)} oncombine={combineAndShare} onbranch={() => putOnBranch(combine!.message)} />
  {/if}

  {#if discardAllOpen}
    <Modal title={t("Discard all your changes?")} onclose={() => (discardAllOpen = false)}>
      <p>{tn(st.changes.length, "Your {n} uncommitted change will be lost: the project folder goes back to the version you're on. This can't be undone.",
        "All {n} uncommitted changes will be lost: the project folder goes back to the version you're on. This can't be undone.")}</p>
      {#snippet footer()}
        <button onclick={() => (discardAllOpen = false)}>{t("Cancel")}</button>
        <button class="danger" onclick={discardEverything}>{t("Discard all")}</button>
      {/snippet}
    </Modal>
  {/if}

  {#if discardSome}
    {@const n = discardSome.length}
    <Modal title={tn(n, "Discard {n} ticked change?", "Discard {n} ticked changes?")} onclose={() => (discardSome = null)}>
      <p>{tn(n, "The file goes back to how it is in the version you're on. The unticked changes stay. This can't be undone.",
        "These {n} files go back to how they are in the version you're on. The unticked changes stay. This can't be undone.")}</p>
      {#snippet footer()}
        <button onclick={() => (discardSome = null)}>{t("Cancel")}</button>
        <button class="danger" onclick={discardTicked}>{t("Discard")}</button>
      {/snippet}
    </Modal>
  {/if}

  {#if restoreFile}
    {@const r = restoreFile}
    {@const pending = st.changes.some((c) => c.path === r.path)}
    <Modal title={t("Restore {file} from “{version}”?", { file: r.path.slice(r.path.lastIndexOf("/") + 1), version: r.label })} onclose={() => (restoreFile = null)}>
      <p>{t("The file goes back to how it was in that version. The rest of the project stays as it is; commit when you're happy with it.")}</p>
      {#if pending}<p class="warn-text">{t("This file has uncommitted changes — they'll be replaced.")}</p>{/if}
      {#snippet footer()}
        <button onclick={() => (restoreFile = null)}>{t("Cancel")}</button>
        <button class="primary" onclick={restoreOneFile}>{t("Restore")}</button>
      {/snippet}
    </Modal>
  {/if}

  {#if discardFile}
    <Modal title={t("Discard your changes to {file}?", { file: discardFile.slice(discardFile.lastIndexOf("/") + 1) })} onclose={() => (discardFile = "")}>
      {@const movedFrom = st?.changes.find((c) => c.path === discardFile)?.from}
      <p>{movedFrom ? t("The file goes back to {path}, as it is in the version you're on. This can't be undone.", { path: movedFrom })
        : t("The file goes back as it is in the version you're on. This can't be undone.")}</p>
      {#snippet footer()}
        <button onclick={() => (discardFile = "")}>{t("Cancel")}</button>
        <button class="danger" onclick={discardOneFile}>{t("Discard changes")}</button>
      {/snippet}
    </Modal>
  {/if}

  {#if discardOpen}
    <Modal title={t("Discard your changes?")} onclose={() => (discardOpen = false)}>
      <p>{tn(st.changes.length, "Your {n} uncommitted change will be lost, and the project gets the team's latest versions of “{branch}”.",
        "Your {n} uncommitted changes will be lost, and the project gets the team's latest versions of “{branch}”.", { branch: st.branch })}</p>
      <p class="muted">{t("To keep them instead, combine them with the team's work or put them on a new branch.")}</p>
      {#snippet footer()}
        <button onclick={() => (discardOpen = false)}>{t("Cancel")}</button>
        <button class="danger" onclick={discardAndUpdate}>{t("Discard and update")}</button>
      {/snippet}
    </Modal>
  {/if}

  {#snippet suggestionText(s: RuleSuggestion)}
    <Tx text={t(s.folder ? "{tool} project found in {folder}." : "{tool} project found in the project folder.")}
      strong={{ folder: `${s.folder}/`, tool: presetName(s.preset) }} />
    <span class="muted" title={s.leftOut.join("  ")}>{s.leftOutBytes > 0
      ? t("Its rules leave out {what} ({size} here).", { what: leftOutText(s.leftOut), size: formatBytes(s.leftOutBytes) })
      : t("Its rules leave out {what}.", { what: leftOutText(s.leftOut) })}</span>
  {/snippet}
  {#snippet suggestionButtons(s: RuleSuggestion)}
    <div class="row">
      <button class="primary" onclick={() => setPreset(s, s.preset)}>{t("Use {tool} rules", { tool: presetName(s.preset) })}</button>
      <button class="ghost" onclick={() => setPreset(s, "none")} title={t("DAWGit won't ask about this folder again")}>{t("Not a project")}</button>
    </div>
  {/snippet}
  {#if rulesAsk && st?.rules.suggestions.length}
    <Modal title={t("Before you commit")} onclose={() => (rulesAsk = false)}>
      <p>{st.rules.suggestions.length === 1 ? t("DAWGit found a project of another tool in this one.") : t("DAWGit found projects of other tools in this one.")}
        {t("A tool's rules leave out what it makes again by itself (caches, backups), so that doesn't go up with the version.")}</p>
      {#each st.rules.suggestions as s (s.folder + s.preset)}
        <div class="suggestion">
          <div>{@render suggestionText(s)}</div>
          {@render suggestionButtons(s)}
        </div>
      {/each}
      {#snippet footer()}
        <button onclick={() => (rulesAsk = false)}>{t("Cancel")}</button>
        <button onclick={() => { rulesAsk = false; commit(false, true); }}>{t("Commit without these rules")}</button>
      {/snippet}
    </Modal>
  {/if}

  {#if untrackedConfirm}
    {@const files = untrackedConfirm}
    <Modal title={commitWarnings.length ? t("Before you commit") : t("No longer tracked")} onclose={() => (untrackedConfirm = null)}>
      {#if commitWarnings.length}
        <ul class="warnings">
          {#each commitWarnings.slice(0, 12) as w}<li>⚠ {w}</li>{/each}
          {#if commitWarnings.length > 12}<li class="faint">… {t("and {n} more", { n: commitWarnings.length - 12 })}</li>{/if}
        </ul>
      {/if}
      {#if files.length}
        <p>{tn(files.length, "The project's rules now leave this file out of versions. It stays on this computer, and on your teammates' computers too.",
          "The project's rules now leave these {n} files out of versions. They stay on this computer, and on your teammates' computers too.")}</p>
        <ul class="untracked mono">
          {#each files.slice(0, 12) as f}<li>{f}</li>{/each}
          {#if files.length > 12}<li class="faint">… {t("and {n} more", { n: files.length - 12 })}</li>{/if}
        </ul>
      {/if}
      {#snippet footer()}
        <button onclick={() => (untrackedConfirm = null)}>{t("Cancel")}</button>
        <button class="primary" onclick={() => { untrackedConfirm = null; commit(true); }}>
          {commitWarnings.length ? t("Commit anyway") : t("Commit")}</button>
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
    <Modal title={isLive || !st?.tool ? (b.set ? t("“{set}” is open in Live", { set: b.set }) : t("Ableton Live is running"))
      : t("{tool} has this project open", { tool: t(st.tool) })} onclose={() => (liveBlocked = null)}>
      {#if isLive || !st?.tool}
        <p>{t("DAWGit is about to change files in this project. Live keeps the open set in memory and would overwrite the changes the next time you save it.")}</p>
        <p class="muted">{t("Save and close the set in Live first — you can leave Live open with another set.")}
          {b.set ? t("Live only shows the set's name, so a set with the same name from another project counts too.")
            : t("DAWGit cannot tell which set Live has open: close Live to go on.")}</p>
      {:else}
        <p>{t("DAWGit is about to change files in this project. {tool} may hold some of them open, or write over the changes.", { tool: t(st.tool) })}</p>
        <p class="muted">{t("Save your work and close the project in {tool} first.", { tool: t(st.tool) })}</p>
      {/if}
      {#snippet footer()}
        <button onclick={() => (liveBlocked = null)}>{t("Cancel")}</button>
        <button class="primary" onclick={() => { const { run: action, resolutions } = b; liveBlocked = null; run(action, resolutions); }}>
          {t("I closed it — continue")}
        </button>
      {/snippet}
    </Modal>
  {/if}

  {#if leaving}
    {@const l = leaving}
    <Modal title={l.target ? t("Go to an older version") : t("Back to the latest version")} onclose={() => (leaving = null)}>
      <p>{tn(st.changes.length, "You have {n} uncommitted change.", "You have {n} uncommitted changes.")}
        {l.target ? t("Going to another version replaces the files in the project folder.") : t("Going back replaces the files in the project folder.")}</p>
      {#if !st.olderVersion}
        <label for="lm">{t("Commit them first as")}</label>
        <input id="lm" bind:value={l.message} placeholder={t("What did you change?")} />
      {:else}
        <p class="muted">{st.remoteUrl ? t("Changes made on an older version can be kept by starting a new branch from here first.")
          : t("Changes made on an older version can be kept by starting a new branch from here or making it the latest version first.")}</p>
      {/if}
      {#snippet footer()}
        <button onclick={() => (leaving = null)}>{t("Cancel")}</button>
        <button class="danger" onclick={discardThenGo}>{t("Discard changes")}</button>
        {#if !st!.olderVersion}
          <button class="primary" disabled={!l.message.trim()} onclick={commitThenGo}>
            {st!.remoteUrl ? t("Commit & share, then go") : t("Commit, then go")}
          </button>
        {/if}
      {/snippet}
    </Modal>
  {/if}

  {#if keepOpen !== null}
    <Modal title={t("Make this the latest version")} onclose={() => (keepOpen = null)}>
      <p class="muted">{t("The older version you are on (with any changes you made) becomes a new version on top of the latest one. Nothing in the history is lost.")}</p>
      <label for="km">{t("Describe it")}</label>
      <input id="km" bind:value={keepOpen} />
      {#snippet footer()}
        <button onclick={() => (keepOpen = null)}>{t("Cancel")}</button>
        <button class="primary" disabled={!keepOpen?.trim() || !!busy} onclick={keepThisVersion}>{t("Make it the latest")}</button>
      {/snippet}
    </Modal>
  {/if}

  {#if newBranch !== null}
    <Modal title={t("New branch")} onclose={() => (newBranch = null)}>
      <p class="muted">{t("A branch is your own line of versions (e.g. to try an idea). The team keeps working on “{branch}”; merge back when you're happy.", { branch: st.branch })}</p>
      <label for="bn">{t("Branch name")}</label>
      <input id="bn" bind:value={newBranch} placeholder="yi-chorus-idea" />
      {#snippet footer()}
        <button onclick={() => (newBranch = null)}>{t("Cancel")}</button>
        <button class="primary" disabled={!newBranch?.trim() || busy === "branch"} onclick={createBranch}>{t("Create")}</button>
      {/snippet}
    </Modal>
  {/if}
{/if}

<style>
  .view { display: flex; flex-direction: column; height: 100%; }
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
  .gear { display: inline-flex; align-items: center; gap: 3px; padding: 3px 6px; font-size: 12.5px; color: var(--faint); }
  .gear svg { width: 15px; height: 15px; }
  .gear:hover { color: var(--text); }
  .gear.bad { color: var(--warn); }
  .untracked { list-style: none; padding: 8px 12px; margin: 10px 0 0; background: var(--bg); border-radius: 8px;
    font-size: 12.5px; max-height: 220px; overflow: auto; }
  .warnings { list-style: none; padding: 0; margin: 0 0 12px; display: flex; flex-direction: column; gap: 6px;
    color: var(--warn); font-size: 13.5px; user-select: text; }

  .banner { display: flex; align-items: center; gap: 10px; margin: 6px 24px; padding: 10px 14px; border-radius: 8px; }
  .banner > div { flex: 1; }
  .banner.info { background: #1d2c38; border: 1px solid #2c4557; }
  .banner.older { background: #2a2536; border: 1px solid #463c5c; }
  .banner.warn { background: var(--warn-bg); border: 1px solid #5a4623; color: #f0d9a8; }
  .suggestion { display: flex; align-items: center; gap: 10px; padding: 10px 0; border-top: 1px solid var(--border); }
  .suggestion > div:first-child { flex: 1; }

  nav { display: flex; gap: 4px; padding: 10px 24px 0; border-bottom: 1px solid var(--line); }
  nav button { border: none; background: transparent; border-radius: 6px 6px 0 0; padding: 8px 14px; color: var(--muted); border-bottom: 2px solid transparent; }
  nav button.on { color: var(--text); border-bottom-color: var(--accent); }
  .count { margin-left: 4px; font-size: 11px; padding: 0 6px; border-radius: 8px; background: #33363d; }

  main { flex: 1; overflow: auto; padding: 16px 24px 32px; }
  .warn-text { color: var(--warn); }
  main.flush { padding: 0; overflow: hidden; min-height: 0; }
  h3 { font-size: 12px; text-transform: uppercase; letter-spacing: .06em; color: var(--muted); margin: 6px 0 10px; }
  .older { margin: 6px 0 0; }
  .commit-btn { width: 100%; margin-top: 8px; padding: 8px 14px; display: flex; align-items: center; justify-content: center; gap: 10px; }
  .commit-btn kbd { font: inherit; font-size: 11px; opacity: .75; padding: 1px 5px; border-radius: 4px; border: 1px solid currentColor; }
  /* nothing to commit yet (no message, no changes): outlined, still easy to see */
  .commit-btn:disabled { background: transparent; border: 1px solid var(--accent); color: var(--accent); opacity: .7; }
  .small { font-size: 12px; margin: 10px 0 0; }

  .tracks { list-style: none; padding: 0; margin: 0 0 18px; display: flex; flex-direction: column; gap: 4px; }
  .tracks li { display: flex; align-items: center; gap: 10px; }
  .chg { width: 8px; height: 8px; border-radius: 2px; background: var(--mod); }
  .chg.added { background: var(--add); }
  .chg.removed { background: var(--del); }
</style>
