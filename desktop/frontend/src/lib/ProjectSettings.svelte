<script lang="ts">
  import { api, errorText, type ProjectInfo, type TeamProject, type TeamSummary } from "./api";
  import Modal from "./Modal.svelte";
  import RulesWindow from "./RulesWindow.svelte";
  import { toast } from "./notify.svelte";

  // A project's settings: its name, where it is, its rules, and what can be
  // done with it (check it, unlink or delete it). The actions that
  // need a confirmation of their own are the caller's.
  let { p, team, onclose, onrenamed, oncheck, ondelete, onunlink, onlocate }: {
    p: TeamProject;
    team?: TeamSummary;     // the project's team (none: on this computer only)
    onclose: () => void;
    onrenamed: () => void;
    oncheck: () => void;
    ondelete: () => void;
    onunlink: () => void;
    onlocate: () => void;
  } = $props();

  const here = $derived(p.status === "downloaded");
  let info = $state<ProjectInfo | null>(null);
  $effect(() => {
    if (!here) return;
    api.ProjectInfo(p.root).then((i) => (info = i)).catch(() => {});
  });

  // svelte-ignore state_referenced_locally
  let name = $state(p.name); // the field starts with the name
  let renaming = $state(false);
  async function rename() {
    renaming = true;
    try {
      await api.RenameProject(team?.id ?? "", p.id, here ? p.root : "", name.trim());
      toast(team ? `Renamed for everyone in ${team.name}` : "Renamed", "ok");
      onrenamed();
    } catch (e) {
      toast(errorText(e), "error");
    } finally {
      renaming = false;
    }
  }

  async function openRules() {
    try {
      await api.OpenRules(p.root);
      toast("Save the file, then DAWGit follows the new rules", "info");
    } catch (e) {
      toast(errorText(e), "error");
    }
  }

  let rulesOpen = $state(false);
  let unlinkSure = $state(false);
  const presetNames: Record<string, string> = { ableton: "Ableton Live project", unity: "Unity project",
    unreal: "Unreal project", design: "Design files", code: "Code", none: "No preset" };
  const presetName = (preset: string) => presetNames[preset] ?? preset;
</script>

{#if rulesOpen}
  <RulesWindow root={p.root} onclose={() => { rulesOpen = false; api.ProjectInfo(p.root).then((i) => (info = i)).catch(() => {}); }} />
{:else}
<Modal title="Project settings" {onclose} width={600}>
  <section>
    <h3>Name</h3>
    <div class="line">
      <input bind:value={name} maxlength="100" aria-label="Project name"
        onkeydown={(e) => { if (e.key === "Enter" && name.trim() && name.trim() !== p.name) rename(); }} />
      <button onclick={rename} disabled={renaming || !name.trim() || name.trim() === p.name}>{renaming ? "Renaming…" : "Rename"}</button>
    </div>
    <p class="hint">{team ? "Project name shared by the whole team." : "Project name in DAWGit."} Local folder keeps its name.</p>
  </section>

  <section>
    <h3>Where</h3>
    <dl>
      <dt>Team</dt><dd>{team ? team.name : "This computer only"}</dd>
      {#if p.root}
        <dt>Folder</dt>
        <dd class="folder">
          <span class="mono path" title={p.root}>{p.root}</span>
          {#if p.status === "missing"}
            <button class="small" onclick={onlocate}>Locate…</button>
          {:else}
            <button class="small" onclick={() => api.ShowFolder(p.root)}>Open folder</button>
          {/if}
        </dd>
      {/if}
      {#if info}<dt>Branch</dt><dd>{info.branch}</dd>{/if}
      {#if p.id}<dt>Project ID</dt><dd class="mono faint">{p.id}</dd>{/if}
    </dl>
  </section>

  {#if here && info}
    <section>
      <h3>Rules</h3>
      <p class="hint">Which files DAWGit tracks, set in the project's <span class="mono">.dawgit.yaml</span>. The file is
        committed with the project, so everyone uses the same rules.</p>
      <ul class="applied">
        {#each info.rules.applied as a}
          <li><strong>{presetName(a.preset)}</strong>
            <span class="faint">{a.folder ? `in ${a.folder}/` : "the project folder"}{a.detected ? " · detected" : ""}</span></li>
        {:else}
          <li class="faint">No preset: every file is tracked.</li>
        {/each}
      </ul>
      {#if info.rules.error}<p class="error">⚠ {info.rules.error}</p>{/if}
      <div class="line">
        <button class="primary" onclick={() => (rulesOpen = true)}>Rules…</button>
        <button onclick={openRules}>Edit as text</button>
        <button class="ghost" onclick={() => api.OpenURL("https://github.com/n0neye/DAWGit/blob/main/docs/profiles.md")}>Guide ↗</button>
      </div>
    </section>

    <section>
      <h3>Health</h3>
      <div class="action">
        <div><strong>Check project…</strong><p class="hint">Reads its whole history again, looking for damage.</p></div>
        <button onclick={oncheck}>Check…</button>
      </div>
    </section>
  {/if}

  <section class="danger-zone">
    <h3>Danger zone</h3>
    {#if p.root}
      <div class="action">
        <div><strong>Unlink folder</strong>
          <p class="hint">DAWGit stops listing this folder{team ? " (the team's copy stays listed, to download)" : ""}.
            Nothing is deleted: the folder keeps its files and versions, and can be added again.</p></div>
        {#if unlinkSure}
          <button class="danger" onclick={onunlink}>Unlink</button>
        {:else}
          <button onclick={() => (unlinkSure = true)}>Unlink…</button>
        {/if}
      </div>
    {/if}
    {#if team}
      <div class="action">
        <div><strong>Delete from {team.name}…</strong>
          <p class="hint">Removes it and all its versions from the team, for everyone.</p></div>
        <button class="danger" onclick={ondelete}>Delete…</button>
      </div>
    {/if}
  </section>

  {#snippet footer()}
    <button onclick={onclose}>Close</button>
  {/snippet}
</Modal>
{/if}

<style>
  section { padding: 12px 0; border-top: 1px solid var(--line); }
  section:first-child { border-top: none; padding-top: 0; }
  h3 { margin: 0 0 8px; font-size: 12px; text-transform: uppercase; letter-spacing: .06em; color: var(--faint); font-weight: 600; }
  .line { display: flex; gap: 8px; align-items: center; }
  .line input { flex: 1; }
  .hint { margin: 6px 0 0; font-size: 12.5px; color: var(--muted); }
  .action .hint { margin-top: 2px; }
  dl { display: grid; grid-template-columns: 90px 1fr; gap: 6px 10px; margin: 0; font-size: 13.5px; align-items: center; }
  dt { color: var(--muted); }
  dd { margin: 0; min-width: 0; }
  .folder { display: flex; gap: 8px; align-items: center; }
  .path { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; min-width: 0; font-size: 12.5px; }
  .small { padding: 2px 10px; font-size: 12.5px; flex: none; }
  .applied { margin: 8px 0; padding-left: 18px; font-size: 13.5px; }
  .error { color: var(--warn); font-size: 13px; margin: 6px 0; }
  .action { display: flex; align-items: center; justify-content: space-between; gap: 16px; padding: 6px 0; }
  .action strong { font-size: 13.5px; font-weight: 600; }
  .action button { flex: none; }
  .danger-zone h3 { color: var(--danger); }
</style>
