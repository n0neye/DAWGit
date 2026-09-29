<script lang="ts">
  import Modal from "./Modal.svelte";
  import { api, errorText, savedAuthor, rememberAuthor, type ProjectSummary, type ServerProject } from "./api";

  let { mode, onadded, onclose }: {
    mode: "folder" | "join";
    onadded: (p: ProjectSummary) => void;
    onclose: () => void;
  } = $props();

  let author = $state(savedAuthor());
  let folder = $state("");
  let url = $state("");
  let token = $state("");
  let serverProjects = $state<ServerProject[] | null>(null);
  let chosen = $state("");
  let parent = $state("");
  let error = $state("");
  let busy = $state(false);

  async function pick(title: string): Promise<string> {
    try {
      return await api.ChooseFolder(title);
    } catch (e) {
      error = errorText(e);
      return "";
    }
  }

  async function add() {
    busy = true;
    error = "";
    try {
      rememberAuthor(author.trim());
      onadded(await api.AddProject(folder, author.trim()));
    } catch (e) {
      error = errorText(e);
    } finally {
      busy = false;
    }
  }

  async function find() {
    busy = true;
    error = "";
    try {
      serverProjects = await api.ServerProjects(url.trim(), token.trim());
      if (serverProjects.length === 1) chosen = serverProjects[0].id;
    } catch (e) {
      error = errorText(e);
    } finally {
      busy = false;
    }
  }

  async function join() {
    busy = true;
    error = "";
    try {
      rememberAuthor(author.trim());
      onadded(await api.JoinProject(url.trim(), token.trim(), chosen, parent, author.trim()));
    } catch (e) {
      error = errorText(e);
    } finally {
      busy = false;
    }
  }
</script>

<Modal title={mode === "folder" ? "Track an Ableton project" : "Join a project from your team server"} {onclose}>
  <label for="author">Your name (shown on your versions)</label>
  <input id="author" bind:value={author} placeholder="e.g. Yi" />

  {#if mode === "folder"}
    <label for="folder">Ableton project folder</label>
    <div class="row">
      <input id="folder" bind:value={folder} placeholder="C:\Music\My Song Project" />
      <button onclick={async () => (folder = (await pick("Choose the Ableton project folder")) || folder)}>Browse…</button>
    </div>
    <p class="faint small">The folder that contains your .als file and the “Ableton Project Info” folder.
      DAWGit keeps its data in a hidden <span class="mono">.dawgit</span> folder inside it.</p>
  {:else}
    <label for="url">Server address or connection code</label>
    <input id="url" bind:value={url} placeholder="http://192.168.0.11:7331  or  dawgit-s3:…" />
    <div class="row find">
      {#if !url.trim().startsWith("dawgit-s3:")}
        <input id="token" bind:value={token} placeholder="Access token" aria-label="Access token" />
      {/if}
      <button disabled={!url.trim() || busy} onclick={find}>Find projects</button>
    </div>
    {#if serverProjects}
      <label for="proj">Project</label>
      {#if serverProjects.length === 0}
        <p class="muted">No projects on this server yet.</p>
      {:else}
        <div class="list" id="proj">
          {#each serverProjects as p (p.id)}
            <button class:on={chosen === p.id} onclick={() => (chosen = p.id)}>{p.name}</button>
          {/each}
        </div>
      {/if}
      <label for="parent">Download into</label>
      <div class="row">
        <input id="parent" bind:value={parent} placeholder="C:\Music" />
        <button onclick={async () => (parent = (await pick("Where should the project folder go?")) || parent)}>Browse…</button>
      </div>
    {/if}
  {/if}

  {#if error}<p class="error">{error}</p>{/if}

  {#snippet footer()}
    <button onclick={onclose}>Cancel</button>
    {#if mode === "folder"}
      <button class="primary" disabled={!folder.trim() || !author.trim() || busy} onclick={add}>Add project</button>
    {:else}
      <button class="primary" disabled={!chosen || !parent.trim() || !author.trim() || busy} onclick={join}>
        {busy ? "Downloading…" : "Join"}
      </button>
    {/if}
  {/snippet}
</Modal>

<style>
  .small { font-size: 12px; }
  .find { margin-top: 8px; }
  .error { color: var(--danger); }
  .list { display: flex; flex-wrap: wrap; gap: 6px; }
  .list button.on { background: var(--accent); color: var(--accent-ink); border-color: var(--accent); }
</style>
