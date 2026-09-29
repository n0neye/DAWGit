<script lang="ts">
  import { untrack } from "svelte";
  import { Events } from "@wailsio/runtime";
  import { api, ago, errorText, progressText, type State, type Result, type Preview, type Conflict, type TeamSummary,
    type Progress } from "./api";
  import { toast } from "./notify.svelte";
  import ChangeList from "./ChangeList.svelte";
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

  let st = $state<State | null>(null);
  let loadError = $state("");
  let tab = $state<"changes" | "history" | "team">("changes");
  let message = $state("");
  let busy = $state("");
  let progress = $state<Progress | null>(null);
  let refreshing = $state(false);

  // dialogs
  let preview = $state<{ title: string; label: string; data: Preview; run: Action; blocked: string } | null>(null);
  const UNSAVED = "You have uncommitted changes. Commit a version instead — the team's changes are merged in as part of it, and anything you both changed is shown then.";
  let conflicts = $state<{ items: Conflict[]; run: Action; force: boolean } | null>(null);
  let liveBlocked = $state<{ run: Action; resolutions: Record<string, string> } | null>(null);
  let shareOpen = $state(false);
  let shareTeam = $state("");
  let branchMenu = $state(false);
  let setMenu = $state(false);
  let newBranch = $state<string | null>(null);

  type Action = { name: string; call: (res: Record<string, string>, force: boolean) => Promise<Result | null>; done: (r: Result) => void };

  async function load() {
    try {
      st = await api.State(root);
      loadError = "";
    } catch (e) {
      loadError = errorText(e);
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

  // Team state (new versions, teammates) is polled slowly...
  $effect(() => {
    const t = setInterval(() => { if (!busy) load(); }, 15000);
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
  let editedTracks = $derived.by(() => {
    const m = new Map<string, string[]>();
    for (const w of st?.teammates ?? []) {
      for (const e of w.edits) {
        // A track someone just created is not the same track as yours even
        // if Live gave both the same id.
        if (!e.track_id || e.change === "added") continue;
        const k = e.set + "|" + e.track_id;
        m.set(k, [...(m.get(k) ?? []), w.author]);
      }
    }
    return m;
  });

  // Runs an action; handles conflicts (ask, retry with decisions) and a
  // running Live (ask, retry with force).
  async function run(a: Action, resolutions: Record<string, string> = {}, force = false) {
    busy = a.name;
    try {
      const r = await a.call(resolutions, force);
      if (!r) return;
      if (r.liveRunning) {
        liveBlocked = { run: a, resolutions };
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

  const saveAction: Action = {
    name: "save",
    call: (res, force) => api.Save(root, message, res, force),
    done: (r) => {
      const text: Record<string, string> = {
        "published": "Version committed and shared with the team",
        "saved-locally": "Version committed on this computer (not shared with a team)",
        "fast-forward": "You had nothing new; updated to the team's latest version",
        "nothing": "Nothing changed since your last version",
      };
      toast(text[r.action] ?? "Version committed", r.action === "nothing" ? "info" : "ok");
      if (r.log.length && r.action === "published") toast("The team's changes were merged into your files — reopen the set in Live", "warn", 9000);
      if (r.action !== "nothing") message = "";
    },
  };

  const updateAction: Action = {
    name: "update",
    call: (res, force) => api.Update(root, res, force),
    done: (r) => {
      if (r.action === "fast-forward" || r.action === "merged") {
        toast("You're up to date — reopen the set in Live to see the changes", "ok", 8000);
        if (r.action === "merged") toast("Your versions and the team's were combined. Commit a version to share the result.", "info", 9000);
      } else toast("Already up to date", "info");
    },
  };

  async function openUpdatePreview() {
    busy = "preview";
    try {
      const data = await api.PreviewUpdate(root);
      if (data) preview = { title: "Updates from the team", label: "Get updates", data, run: updateAction,
        blocked: st?.changes.length ? UNSAVED : "" };
    } catch (e) {
      toast(errorText(e), "error");
    } finally {
      busy = "";
    }
  }

  async function openMergePreview(name: string) {
    branchMenu = false;
    busy = "preview";
    try {
      const data = await api.PreviewMerge(root, name);
      if (!data) return;
      preview = {
        title: `Merge “${name}” into “${st?.branch}”`, label: "Merge and share", data,
        blocked: st?.changes.length ? "You have uncommitted changes. Commit a version first, then merge." : "",
        run: {
          name: "merge",
          call: (res, force) => api.MergeBranch(root, name, res, force),
          done: (r) => toast(r.action === "up-to-date" || r.action === "ahead"
            ? `Nothing to merge from ${name}` : `Merged ${name} into ${st?.branch} and shared it — reopen the set in Live`, "ok", 8000),
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
      done: () => toast(`Now working on “${name}” — reopen the set in Live`, "ok", 8000),
    });
  }

  async function createBranch() {
    const name = (newBranch ?? "").trim();
    if (!name) return;
    busy = "branch";
    try {
      await api.CreateBranch(root, name);
      toast(`Created “${name}”. Versions you save now go there.`, "ok");
      newBranch = null;
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
  function shareFirstVersion(team = "the team") {
    onfirstshared?.(); // started: don't start again if this view is reopened
    run({
      name: "first-share",
      call: (res, force) => api.Save(root, "First version", res, force),
      done: () => {
        toast(`“${st?.name ?? folderName}” is shared with ${team}`, "ok");
      },
    });
  }

  let folderName = $derived(root.split(/[\\/]/).pop()?.replace(/ Project$/, "") ?? root);

  function editors(set: string, trackId: string | undefined, change: string): string[] {
    if (!trackId || change === "added") return [];
    return editedTracks.get(set + "|" + trackId) ?? [];
  }
</script>

<svelte:window onfocus={() => { if (!busy) load(); }} onclick={(e) => {
  const t = e.target as HTMLElement;
  if (branchMenu && !t.closest(".branch-wrap")) branchMenu = false;
  if (setMenu && !t.closest(".open-wrap")) setMenu = false;
}} />

{#snippet progressBar(p: Progress, team?: string)}
  <div class="progress">
    <span>{progressText(p, team)}</span>
    {#if p.total}<div class="bar"><div style="width: {Math.round((100 * p.done) / p.total)}%"></div></div>{/if}
  </div>
{/snippet}

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
    {#if progress}{@render progressBar(progress)}{/if}
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
            <span class="dot" class:on={st.online}></span>
            <span class="faint" title={st.online ? st.remoteUrl : st.offline}>
              {st.teamName || st.remoteUrl}{st.online ? "" : " · not reachable"}
            </span>
          {:else if teams.length}
            <button class="ghost" onclick={openShare}>Share with a team…</button>
          {:else}
            <span class="faint">on this computer only</span>
          {/if}
        </div>
      </div>
      <div class="actions">
        {#if st.sets.length === 1}
          <button onclick={() => api.OpenInLive(st!.root, st!.sets[0])} title="Open {st.sets[0]} in Ableton Live">▶ Open in Live</button>
        {:else if st.sets.length > 1}
          <div class="open-wrap">
            <button onclick={() => (setMenu = !setMenu)} title="Open a set in Ableton Live">▶ Open in Live ▾</button>
            {#if setMenu}
              <div class="menu right" role="menu">
                {#each st.sets as s}
                  <button class="item" onclick={() => { setMenu = false; api.OpenInLive(st!.root, s); }}>{s}</button>
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
      <div class="banner info">{@render progressBar(progress, st.teamName || undefined)}</div>
    {:else if busy === "first-share"}
      <div class="banner info"><div>Sharing “{st.name}” with the team…</div></div>
    {/if}

    {#if st.incoming.length}
      <div class="banner info">
        <div>
          <strong>{[...new Set(st.incoming.map((v) => v.author))].join(", ")}</strong>
          saved {st.incoming.length} new version{st.incoming.length === 1 ? "" : "s"}:
          <span class="muted">{st.incoming.slice(0, 3).map((v) => `“${v.message}”`).join(", ")}{st.incoming.length > 3 ? "…" : ""}</span>
        </div>
        <button onclick={openUpdatePreview} disabled={!!busy}>Preview</button>
        <button class="primary" onclick={() => run(updateAction)} disabled={!!busy || st.changes.length > 0}
          title={st.changes.length ? "You have uncommitted changes: commit a version to get these too" : ""}>Get updates</button>
      </div>
    {/if}
    {#each st.overlaps as o}
      <div class="banner warn">⚠ {o[0].toUpperCase() + o.slice(1)} — talk before you both save.</div>
    {/each}

    <nav>
      <button class:on={tab === "changes"} onclick={() => (tab = "changes")}>
        Changes {#if st.changes.length}<span class="count">{st.changes.length}</span>{/if}
      </button>
      <button class:on={tab === "history"} onclick={() => (tab = "history")}>History</button>
      <button class:on={tab === "team"} onclick={() => (tab = "team")} disabled={!st.remoteUrl}>
        Team {#if st.teammates.length}<span class="count">{st.teammates.length}</span>{/if}
      </button>
    </nav>

    <main>
      {#if tab === "changes"}
        <div class="changes">
          <section>
            {#if st.myEdits.length}
              <h3>Tracks you changed</h3>
              <ul class="tracks">
                {#each st.myEdits as e}
                  {@const who = editors(e.set, e.track_id, e.change)}
                  <li>
                    <span class="chg {e.change}"></span>
                    <span>{e.name}</span>
                    <span class="faint">{e.set}</span>
                    {#if who.length}<span class="lock" title="Also being edited">✎ {who.join(", ")}</span>{/if}
                  </li>
                {/each}
              </ul>
            {/if}
            <h3>Files</h3>
            <ChangeList changes={st.changes}
              empty="No uncommitted changes. Work in Live and press Ctrl+S — your changes show up here." />
          </section>
        </div>
      {:else if tab === "history"}
        <History versions={st.history} head={st.head} incoming={incomingIds} />
      {:else}
        <section>
          <h3>Being edited right now (not committed yet)</h3>
          {#each st.teammates as w (w.author)}
            <div class="mate">
              <div class="row"><strong>{w.author}</strong><span class="faint">updated {ago(w.updated)}</span></div>
              <ul class="tracks">
                {#each w.edits as e}
                  <li><span class="chg {e.change}"></span><span>{e.name}</span><span class="faint">{e.set}</span></li>
                {/each}
              </ul>
            </div>
          {:else}
            <p class="muted">Nobody else has uncommitted work at the moment.</p>
          {/each}
        </section>
      {/if}
    </main>

    {#if tab === "changes"}
      <footer class="save">
        <textarea rows="2" bind:value={message} placeholder="What did you change? e.g. “New bassline in the chorus”"
          onkeydown={(e) => { if (e.key === "Enter" && e.ctrlKey && message.trim() && !busy) run(saveAction); }}></textarea>
        <div class="save-row">
          <p class="faint small">
            {#if st.remoteUrl}
              Commits the project folder and shares it with the team on “{st.branch}”. If others saved in the
              meantime, their changes are merged in first.
            {:else}
              Commits on this computer. Share the project with a team to work on it together.
            {/if}
          </p>
          <button class="primary" disabled={!message.trim() || !!busy} onclick={() => run(saveAction)}
            title="Ctrl+Enter">
            {busy === "save" || busy === "first-share" ? "Committing…" : st.remoteUrl ? "Commit version & share" : "Commit version"}
          </button>
        </div>
      </footer>
    {/if}
  </div>

  {#if preview}
    {@const p = preview}
    <PreviewDialog title={p.title} preview={p.data} actionLabel={p.label} blocked={p.blocked}
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
    <Modal title="Ableton Live is running" onclose={() => (liveBlocked = null)}>
      <p>DAWGit is about to change files in this project. If the set is open in Live, Live keeps the old
        version in memory and would overwrite the changes the next time you save.</p>
      <p class="muted">Save and close the set in Live first (you can leave Live open with another set).</p>
      {#snippet footer()}
        <button onclick={() => (liveBlocked = null)}>Cancel</button>
        <button class="primary" onclick={() => { const { run: action, resolutions } = b; liveBlocked = null; run(action, resolutions, true); }}>
          The set is closed — continue
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
  .preparing { padding: 60px 40px; max-width: 560px; }
  .preparing h1 { font-size: 22px; margin: 0 0 8px; }
  .progress { flex: 1; display: flex; flex-direction: column; gap: 6px; font-size: 13px; }
  .bar { height: 4px; border-radius: 2px; background: var(--line); overflow: hidden; }
  .bar > div { height: 100%; background: var(--accent); transition: width .2s; }
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

  .banner { display: flex; align-items: center; gap: 10px; margin: 6px 24px; padding: 10px 14px; border-radius: 8px; }
  .banner > div { flex: 1; }
  .banner.info { background: #1d2c38; border: 1px solid #2c4557; }
  .banner.warn { background: var(--warn-bg); border: 1px solid #5a4623; color: #f0d9a8; }

  nav { display: flex; gap: 4px; padding: 10px 24px 0; border-bottom: 1px solid var(--line); }
  nav button { border: none; background: transparent; border-radius: 6px 6px 0 0; padding: 8px 14px; color: var(--muted); border-bottom: 2px solid transparent; }
  nav button.on { color: var(--text); border-bottom-color: var(--accent); }
  .count { margin-left: 4px; font-size: 11px; padding: 0 6px; border-radius: 8px; background: #33363d; }

  main { flex: 1; overflow: auto; padding: 16px 24px 32px; }
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
  .lock { font-size: 12px; color: var(--warn); background: var(--warn-bg); padding: 0 8px; border-radius: 8px; }
  .mate { padding: 12px 14px; margin-bottom: 10px; background: var(--panel); border: 1px solid var(--line); border-radius: 10px; }
  .mate .tracks { margin: 8px 0 0; }
</style>
