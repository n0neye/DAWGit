<script lang="ts">
  import { onMount } from "svelte";
  import { Events } from "@wailsio/runtime";
  import { api, errorText, type Overview, type TeamProject } from "./lib/api";
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

  const SELECTED_KEY = "dawgit.selected";
  const DOWNLOAD_DIR_KEY = "dawgit.downloadDir";
  const remember = (k: string, v: string) => { try { localStorage.setItem(k, v); } catch { /* not persisted */ } };
  const recall = (k: string) => { try { return localStorage.getItem(k) ?? ""; } catch { return ""; } };

  async function reload() {
    try {
      overview = await api.Overview();
    } catch (e) {
      toast(errorText(e), "error");
    }
  }

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

  async function addToTeam() {
    const folder = await api.ChooseFolder(`Choose an Ableton project folder to share with ${current?.name ?? "the team"}`);
    if (folder) confirmShare = folder;
  }

  async function share(folder: string) {
    confirmShare = null;
    busy = "add";
    try {
      await api.AddProjectToTeam(overview!.currentTeam, folder, false);
      toast(`Shared with ${current?.name}`, "ok");
      await reload();
      const got = overview?.projects.find((p) => p.root === folder);
      if (got) select(got);
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
    return Events.On("agent", (ev: { data: AgentEvent }) => {
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
  });

  const statusText: Record<string, string> = { remote: "not downloaded", missing: "folder not found", local: "not shared" };
  const statusIcon: Record<string, string> = { remote: "☁", missing: "⚠", downloaded: "♪", local: "♪" };
</script>

{#if !overview}
  <div class="loading faint">Loading…</div>
{:else if onboarding}
  <Onboarding {overview} {reload} onfinish={async (root) => {
    onboarding = false;
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
          <div class="section">Projects</div>
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
          <button class="add" onclick={addToTeam} disabled={busy === "add"}>+ Add project folder</button>
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
          <ProjectView root={selectedEntry.root} {refreshKey} teams={overview.teams} onchanged={reload} />
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
          <p class="muted">{current ? "Pick a song on the left, or add a project folder to share it with the team." : "Connect to a team from the menu at the top left."}</p>
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
        <span class="meta">{statusText[p.status] ?? `⑂ ${p.branch}`}</span>
      </span>
    </button>
    {#if p.root && p.status !== "missing"}
      <button class="ghost rm" title="Remove from this computer's list (files stay)" onclick={() => forget(p)}>✕</button>
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

<Toasts />

<style>
  .loading { padding: 40px; }
  .shell { display: grid; grid-template-columns: 250px 1fr; height: 100%; }
  aside { background: #141518; border-right: 1px solid var(--line); display: flex; flex-direction: column; padding: 12px 10px; min-height: 0; }
  .brand { display: flex; align-items: center; gap: 8px; font-weight: 700; font-size: 15px; padding: 2px 8px 10px; }
  .brand img { width: 20px; height: 20px; }
  .list { flex: 1; overflow: auto; min-height: 0; }
  .section { font-size: 11px; text-transform: uppercase; letter-spacing: .06em; color: var(--faint); padding: 8px 8px 4px; }
  ul { list-style: none; margin: 0; padding: 0; }
  li { display: flex; align-items: center; }
  .proj { flex: 1; min-width: 0; display: flex; align-items: center; gap: 10px; border: none; background: transparent; padding: 7px 10px; border-radius: 8px; text-align: left; }
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
  .rm { visibility: hidden; padding: 2px 6px; }
  li:hover .rm { visibility: visible; }
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
