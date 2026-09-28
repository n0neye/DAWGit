<script lang="ts">
  import { onMount } from "svelte";
  import { Events } from "@wailsio/runtime";
  import { api, errorText, type ProjectSummary } from "./lib/api";
  import { toast } from "./lib/notify.svelte";
  import ProjectView from "./lib/ProjectView.svelte";
  import AddProjectDialog from "./lib/AddProjectDialog.svelte";
  import Toasts from "./lib/Toasts.svelte";

  let projects = $state<ProjectSummary[]>([]);
  let selected = $state("");
  let adding = $state<"folder" | "join" | null>(null);
  let refreshKey = $state(0);

  const SELECTED_KEY = "dawgit.selected";

  async function loadProjects() {
    try {
      projects = await api.Projects();
      if (!projects.some((p) => p.root === selected)) {
        let last = "";
        try { last = localStorage.getItem(SELECTED_KEY) ?? ""; } catch { /* ignore */ }
        selected = projects.find((p) => p.root === last)?.root ?? projects[0]?.root ?? "";
      }
    } catch (e) {
      toast(errorText(e), "error");
    }
  }

  function select(root: string) {
    selected = root;
    try { localStorage.setItem(SELECTED_KEY, root); } catch { /* ignore */ }
  }

  async function remove(p: ProjectSummary) {
    await api.RemoveProject(p.root);
    toast(`Removed “${p.name}” from the list (files untouched)`, "info");
    await loadProjects();
  }

  type AgentEvent = { root: string; kind: string; author: string; labels: string[]; text: string; versions: { author: string; message: string }[] };

  onMount(() => {
    loadProjects();
    return Events.On("agent", (ev: { data: AgentEvent }) => {
      const e = ev.data;
      const name = projects.find((p) => p.root === e.root)?.name ?? "";
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
      if (e.root === selected && e.kind !== "backed-up") refreshKey++;
    });
  });
</script>

<div class="shell">
  <aside>
    <div class="brand"><img src="/icon.png" alt="" /> DAWGit</div>
    <div class="section">Projects</div>
    <ul>
      {#each projects as p (p.root)}
        <li>
          <button class="proj" class:on={p.root === selected} onclick={() => select(p.root)} title={p.root}>
            <span class="name">{p.name}</span>
            <span class="meta">{p.error ? "unavailable" : p.remoteUrl ? `⑂ ${p.branch}` : "not shared"}</span>
          </button>
          <button class="ghost rm" title="Remove from list" onclick={() => remove(p)}>✕</button>
        </li>
      {/each}
    </ul>
    <div class="add">
      <button onclick={() => (adding = "folder")}>+ Add project folder</button>
      <button class="ghost" onclick={() => (adding = "join")}>Join from team server</button>
    </div>
  </aside>

  <section class="content">
    {#if selected}
      {#key selected}
        <ProjectView root={selected} {refreshKey} onchanged={loadProjects} />
      {/key}
    {:else}
      <div class="empty">
        <h1>Welcome to DAWGit</h1>
        <p class="muted">Version history and teamwork for your Ableton Live projects.</p>
        <div class="row">
          <button class="primary" onclick={() => (adding = "folder")}>Add a project folder</button>
          <button onclick={() => (adding = "join")}>Join from team server</button>
        </div>
      </div>
    {/if}
  </section>
</div>

{#if adding}
  <AddProjectDialog mode={adding} onclose={() => (adding = null)}
    onadded={async (p) => { adding = null; await loadProjects(); select(p.root); toast(`Added “${p.name}”`, "ok"); }} />
{/if}

<Toasts />

<style>
  .shell { display: grid; grid-template-columns: 240px 1fr; height: 100%; }
  aside { background: #141518; border-right: 1px solid var(--line); display: flex; flex-direction: column; padding: 14px 10px; }
  .brand { display: flex; align-items: center; gap: 8px; font-weight: 700; font-size: 16px; padding: 4px 8px 14px; }
  .brand img { width: 22px; height: 22px; }
  .section { font-size: 11px; text-transform: uppercase; letter-spacing: .06em; color: var(--faint); padding: 6px 8px; }
  ul { list-style: none; margin: 0; padding: 0; flex: 1; overflow: auto; }
  li { display: flex; align-items: center; }
  .proj { flex: 1; min-width: 0; display: flex; flex-direction: column; align-items: flex-start; border: none; background: transparent; padding: 8px 10px; border-radius: 8px; text-align: left; }
  .proj:hover { background: var(--panel); }
  .proj.on { background: var(--panel-2); }
  .name { font-weight: 600; max-width: 100%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .meta { font-size: 12px; color: var(--faint); }
  .rm { visibility: hidden; padding: 2px 6px; }
  li:hover .rm { visibility: visible; }
  .add { display: flex; flex-direction: column; gap: 6px; padding-top: 10px; border-top: 1px solid var(--line); }
  .content { min-width: 0; overflow: hidden; }
  .empty { padding: 64px; }
  .empty h1 { margin: 0 0 6px; }
</style>
