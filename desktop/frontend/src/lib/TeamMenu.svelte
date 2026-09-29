<script lang="ts">
  import { api, errorText, LOCAL, type Overview, type TeamSummary } from "./api";
  import { toast } from "./notify.svelte";
  import Modal from "./Modal.svelte";
  import ConnectForm from "./ConnectForm.svelte";

  let { overview, reload }: { overview: Overview; reload: () => Promise<void> } = $props();

  let open = $state(false);
  let connecting = $state(false);
  let managing = $state(false);
  let names = $state<Record<string, string>>({});
  let confirmRemove = $state<TeamSummary | null>(null);

  let current = $derived(overview.teams.find((t) => t.id === overview.currentTeam));
  const hostOf = (t: TeamSummary) => t.address.replace(/^https?:\/\//, "");
  // "Local" (projects kept on this computer only) sits in the menu like a team.
  let isLocal = $derived(!current);

  async function select(id: string) {
    open = false;
    try {
      await api.SelectTeam(id);
      await reload();
    } catch (e) {
      toast(errorText(e), "error");
    }
  }

  async function connected(t: TeamSummary) {
    connecting = false;
    toast(`Connected to ${t.name}`, "ok");
    await reload();
  }

  function manage() {
    open = false;
    names = Object.fromEntries(overview.teams.map((t) => [t.id, t.name]));
    managing = true;
  }

  async function rename(t: TeamSummary) {
    try {
      await api.RenameTeam(t.id, names[t.id]);
      await reload();
      toast("Renamed", "ok");
    } catch (e) {
      toast(errorText(e), "error");
    }
  }

  async function remove(t: TeamSummary) {
    confirmRemove = null;
    try {
      await api.RemoveTeam(t.id);
      await reload();
      toast(`Disconnected from ${t.name}. Project folders were left on disk.`, "info", 7000);
    } catch (e) {
      toast(errorText(e), "error");
    }
  }
</script>

<svelte:window onclick={(e) => { if (open && !(e.target as HTMLElement).closest(".team-menu")) open = false; }} />

<div class="team-menu">
  <button class="current" onclick={() => (open = !open)} title={current?.address ?? "Projects kept on this computer only"}>
    <span class="label">{isLocal ? "Local" : "Team"}</span>
    <span class="name">{current?.name ?? "This computer"}</span>
    <span class="caret">▾</span>
  </button>
  {#if open}
    <div class="menu" role="menu">
      {#each overview.teams as t (t.id)}
        <button class="item" onclick={() => select(t.id)}>
          <span class="check">{t.id === overview.currentTeam ? "✓" : ""}</span>
          <span class="tname">{t.name}</span>
          <span class="faint small">{t.isStorage ? "storage" : hostOf(t) === t.name ? "" : hostOf(t)}</span>
        </button>
      {/each}
      {#if overview.teams.length}<div class="sep"></div>{/if}
      <button class="item" onclick={() => select(LOCAL)}>
        <span class="check">{isLocal ? "✓" : ""}</span>
        <span class="tname">Local</span>
        <span class="faint small">this computer only</span>
      </button>
      <div class="sep"></div>
      <button class="item" onclick={() => { open = false; connecting = true; }}>
        <span class="check">+</span>Connect to {overview.teams.length ? "another" : "a"} team…
      </button>
      {#if overview.teams.length}
        <button class="item" onclick={manage}><span class="check">⚙</span>Manage teams…</button>
      {/if}
    </div>
  {/if}
</div>

{#if connecting}
  <Modal title="Connect to a team" onclose={() => (connecting = false)}>
    <ConnectForm onconnected={connected} />
  </Modal>
{/if}

{#if managing}
  <Modal title="Teams on this computer" onclose={() => (managing = false)} width={620}>
    <ul class="teams">
      {#each overview.teams as t (t.id)}
        <li>
          <div class="fields">
            <input bind:value={names[t.id]} aria-label="Team name" />
            <div class="faint small mono">{t.address}</div>
          </div>
          <button disabled={!names[t.id]?.trim() || names[t.id] === t.name} onclick={() => rename(t)}>Rename</button>
          <button class="danger" onclick={() => (confirmRemove = t)}>Disconnect</button>
        </li>
      {/each}
    </ul>
    {#snippet footer()}
      <button onclick={() => (managing = false)}>Close</button>
    {/snippet}
  </Modal>
{/if}

{#if confirmRemove}
  {@const t = confirmRemove}
  <Modal title="Disconnect from {t.name}?" onclose={() => (confirmRemove = null)}>
    <p>This computer forgets the team and its access token or key. Your project folders stay where they are,
      but DAWGit stops listing and watching them. You can connect again later.</p>
    {#snippet footer()}
      <button onclick={() => (confirmRemove = null)}>Cancel</button>
      <button class="primary" onclick={() => remove(t)}>Disconnect</button>
    {/snippet}
  </Modal>
{/if}

<style>
  .team-menu { position: relative; margin-bottom: 10px; }
  .current {
    width: 100%; display: grid; grid-template-columns: 1fr auto; grid-template-rows: auto auto;
    text-align: left; padding: 8px 10px; background: var(--panel); border-radius: 8px;
  }
  .label { grid-column: 1; font-size: 11px; text-transform: uppercase; letter-spacing: .06em; color: var(--faint); }
  .name { grid-column: 1; font-weight: 650; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .caret { grid-column: 2; grid-row: 1 / 3; align-self: center; color: var(--muted); }
  .menu {
    position: absolute; top: calc(100% + 4px); left: 0; right: -60px; z-index: 30; padding: 6px;
    background: var(--panel-2); border: 1px solid var(--line); border-radius: 8px; box-shadow: 0 12px 30px rgba(0, 0, 0, .45);
  }
  .item { display: flex; align-items: center; gap: 8px; width: 100%; border: none; background: transparent; padding: 7px 8px; text-align: left; }
  .item:hover { background: #33363d; }
  .check { width: 14px; color: var(--accent); }
  .tname { flex: 1; }
  .small { font-size: 12px; }
  .sep { height: 1px; background: var(--line); margin: 6px 0; }
  .teams { list-style: none; padding: 0; margin: 0; display: flex; flex-direction: column; gap: 10px; }
  .teams li { display: flex; align-items: center; gap: 8px; }
  .fields { flex: 1; min-width: 0; }
  .fields .mono { margin-top: 3px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
</style>
