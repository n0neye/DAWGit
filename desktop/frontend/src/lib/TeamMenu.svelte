<script lang="ts">
  import { api, errorText, LOCAL, type Overview, type TeamSummary } from "./api";
  import { toast } from "./notify.svelte";
  import Modal from "./Modal.svelte";
  import ConnectForm from "./ConnectForm.svelte";
  import IdentityForm from "./IdentityForm.svelte";

  let { overview, reload }: { overview: Overview; reload: () => Promise<void> } = $props();

  let open = $state(false);
  let connecting = $state(false);
  let managing = $state(false);
  let names = $state<Record<string, string>>({});
  let confirmRemove = $state<TeamSummary | null>(null);
  // Who you are in a team (after connecting, or to rename yourself).
  let identityFor = $state<TeamSummary | null>(null);
  // Your name for projects kept on this computer only.
  let localName = $state<string | null>(null);

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
    if (!t.memberId) identityFor = t;
  }

  async function identitySaved(t: TeamSummary, renamed: boolean) {
    identityFor = null;
    await reload();
    toast(renamed ? `You're “${t.memberName}” in ${t.name} — on all your versions` : `You're “${t.memberName}” in ${t.name}`, "ok");
  }

  async function saveLocalName() {
    const n = (localName ?? "").trim();
    localName = null;
    try {
      await api.SetAuthor(n);
      await reload();
    } catch (e) {
      toast(errorText(e), "error");
    }
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
      names[t.id] = overview.teams.find((x) => x.id === t.id)?.name ?? names[t.id];
      toast("Renamed", "ok");
    } catch (e) {
      toast(errorText(e), "error");
    }
  }

  let renaming = $state("");
  async function renameForEveryone(t: TeamSummary) {
    renaming = t.id;
    try {
      await api.RenameTeamForEveryone(t.id, names[t.id]);
      await reload();
      toast(`Renamed to ${names[t.id].trim()} for everyone`, "ok");
    } catch (e) {
      toast(errorText(e), "error", 9000);
    } finally {
      renaming = "";
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
  {#if current && !current.memberId}
    <button class="who" onclick={() => (identityFor = current!)}
      title="Versions you commit here show this name, for everyone in the team">
      ☺ Choose your name in {current.name}
    </button>
  {/if}
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
      {#if current}
        <button class="item" onclick={() => { open = false; identityFor = current!; }}>
          <span class="check">☺</span>Your name in {current.name}{current.memberName ? `: ${current.memberName}` : "…"}
        </button>
      {:else}
        <button class="item" onclick={() => { open = false; localName = overview.author; }}>
          <span class="check">☺</span>Your name on this computer{overview.author ? `: ${overview.author}` : "…"}
        </button>
      {/if}
      <button class="item" onclick={() => { open = false; connecting = true; }}>
        <span class="check">+</span>Connect to {overview.teams.length ? "another" : "a"} team…
      </button>
      {#if overview.teams.length}
        <button class="item" onclick={manage}><span class="check">⚙</span>Manage teams…</button>
      {/if}
    </div>
  {/if}
</div>

{#if identityFor}
  {@const t = identityFor}
  <Modal title={t.memberId ? `Your name in ${t.name}` : `Who are you in ${t.name}?`} onclose={() => (identityFor = null)}>
    <IdentityForm team={t} suggested={overview.author} submitLabel="Save"
      onsaved={(saved) => identitySaved(saved, !!t.memberId && saved.memberName !== t.memberName)} />
  </Modal>
{/if}

{#if localName !== null}
  <Modal title="Your name on this computer" onclose={() => (localName = null)}>
    <p class="muted">Used for projects kept on this computer only, and suggested when you join a team.</p>
    <input bind:value={localName} placeholder="e.g. Yi" />
    {#snippet footer()}
      <button onclick={() => (localName = null)}>Cancel</button>
      <button class="primary" disabled={!localName?.trim()} onclick={saveLocalName}>Save</button>
    {/snippet}
  </Modal>
{/if}

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
            <input bind:value={names[t.id]} aria-label="Team name" placeholder="The team's own name" />
            <div class="faint small mono">{t.address}</div>
            {#if names[t.id] !== t.name}
              <div class="rename">
                {#if names[t.id]?.trim()}
                  <button class="primary" disabled={!!renaming} onclick={() => renameForEveryone(t)}>
                    {renaming === t.id ? "Renaming…" : "Rename for everyone"}
                  </button>
                  <button disabled={!!renaming} onclick={() => rename(t)}>Only on this computer</button>
                {:else}
                  <button onclick={() => rename(t)}>Use the team's name</button>
                {/if}
                <button class="ghost" onclick={() => (names[t.id] = t.name)}>Cancel</button>
              </div>
            {/if}
          </div>
          <button class="danger" onclick={() => (confirmRemove = t)}>Disconnect</button>
        </li>
      {/each}
    </ul>
    <p class="faint small note"><strong>Rename for everyone</strong> changes the team's name on its server or storage,
      and every member sees the new name. <strong>Only on this computer</strong> keeps your own name for it here.</p>
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
  .who {
    width: 100%; margin-top: 6px; padding: 5px 10px; font-size: 12.5px; text-align: left;
    background: var(--warn-bg); color: var(--warn); border: 1px solid #5a4623; border-radius: 8px;
  }
  .sep { height: 1px; background: var(--line); margin: 6px 0; }
  .teams { list-style: none; padding: 0; margin: 0; display: flex; flex-direction: column; gap: 10px; }
  .teams li { display: flex; align-items: flex-start; gap: 8px; }
  .rename { display: flex; gap: 6px; margin-top: 8px; }
  .rename button { padding: 5px 10px; font-size: 13px; }
  .fields { flex: 1; min-width: 0; }
  .note { margin: 14px 0 0; }
  .fields .mono { margin-top: 3px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
</style>
