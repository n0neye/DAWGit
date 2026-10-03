<script lang="ts">
  import { t as tr, tn } from "./i18n.svelte"; // t is a team here
  import { api, errorText, type Overview, type TeamSummary } from "./api";
  import { toast } from "./notify.svelte";
  import Modal from "./Modal.svelte";
  import JoinOrCreate from "./JoinOrCreate.svelte";
  import TeamSettings from "./TeamSettings.svelte";
  import IdentityForm from "./IdentityForm.svelte";

  let { overview, reload }: { overview: Overview; reload: () => Promise<void> } = $props();

  let open = $state(false);
  let connecting = $state(false);
  let settingsFor = $state<TeamSummary | null>(null); // the ⚙ of a team
  // Who you are in a team (after connecting, or to rename yourself).
  let identityFor = $state<TeamSummary | null>(null);

  let current = $derived(overview.teams.find((t) => t.id === overview.currentTeam));
  const hostOf = (t: TeamSummary) => t.address.replace(/^https?:\/\//, "");

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
    toast(tr("Connected to {team}", { team: t.name }), "ok");
    await reload();
    if (!t.memberId) {
      identityFor = t;
      offerAfterName = t;
    } else {
      offerReconnect(t);
    }
  }

  async function identitySaved(t: TeamSummary, renamed: boolean) {
    identityFor = null;
    await reload();
    toast(tr(renamed ? "You're “{name}” in {team} — on all your versions" : "You're “{name}” in {team}", { name: t.memberName, team: t.name }), "ok");
    if (offerAfterName) {
      const joined = offerAfterName;
      offerAfterName = null;
      offerReconnect(joined);
    }
  }

  // Projects of the team already on this computer (e.g. from before it
  // disconnected): offered for reconnecting, ticked by default.
  let offerAfterName: TeamSummary | null = null;
  let found = $state<{ team: TeamSummary; projects: { root: string; name: string; on: boolean }[] } | null>(null);
  let reconnecting = $state(false);

  async function offerReconnect(t: TeamSummary) {
    try {
      const ps = (await api.TeamProjectsHere(t.id)) ?? [];
      if (ps.length) found = { team: t, projects: ps.map((p) => ({ ...p, on: true })) };
    } catch {
      // not reachable now: they can still be added by hand
    }
  }

  async function reconnect() {
    if (!found) return;
    const f = found;
    const roots = f.projects.filter((p) => p.on).map((p) => p.root);
    reconnecting = true;
    try {
      await api.ReconnectProjects(f.team.id, roots);
      found = null;
      await reload();
      toast(tn(roots.length, "Reconnected {n} project to {team}", "Reconnected {n} projects to {team}", { team: f.team.name }), "ok");
    } catch (e) {
      toast(errorText(e), "error", 9000);
    } finally {
      reconnecting = false;
    }
  }
</script>

<svelte:window onclick={(e) => { if (open && !(e.target as HTMLElement).closest(".team-menu")) open = false; }} />

<div class="team-menu">
  <button class="current" onclick={() => (open = !open)} title={current?.address ?? ""}>
    <span class="label">{tr("Team")}</span>
    <span class="name">{current?.name ?? tr("No team")}</span>
    <span class="caret">▾</span>
  </button>
  {#if current && !current.memberId}
    <button class="who" onclick={() => (identityFor = current!)}
      title={tr("Versions you commit here show this name, for everyone in the team")}>
      ☺ {tr("Choose your name in {team}", { team: current.name })}
    </button>
  {/if}
  {#if open}
    <div class="menu" role="menu">
      {#each overview.teams as t (t.id)}
        <div class="team-row">
          <button class="item" onclick={() => select(t.id)}>
            <span class="check">{t.id === overview.currentTeam ? "✓" : ""}</span>
            <span class="tname">{t.name}</span>
            {#if !t.isStorage && hostOf(t) !== t.name}<span class="faint small">{hostOf(t)}</span>{/if}
          </button>
          <button class="gear" title={tr("Team settings: names, connection code, keys")}
            onclick={() => { open = false; settingsFor = t; }}>⚙</button>
        </div>
      {/each}
      {#if overview.teams.length}<div class="sep"></div>{/if}
      <button class="item" onclick={() => { open = false; connecting = true; }}>
        <span class="check">+</span>{tr("Join/Create a Team…")}
      </button>
    </div>
  {/if}
</div>

{#if identityFor}
  {@const t = identityFor}
  <Modal title={t.memberId ? tr("Your name in {team}", { team: t.name }) : tr("Who are you in {team}?", { team: t.name })} onclose={() => (identityFor = null)}>
    <IdentityForm team={t} suggested={overview.author} submitLabel={tr("Save")} askShare
      onsaved={(saved) => identitySaved(saved, !!t.memberId && saved.memberName !== t.memberName)} />
  </Modal>
{/if}

{#if connecting}
  <Modal title={tr("Join or create a team")} onclose={() => (connecting = false)} width={640} backdropCloses={false}>
    <JoinOrCreate onconnected={connected} />
  </Modal>
{/if}

{#if found}
  {@const f = found}
  <Modal title={tr("Projects of {team} on this computer", { team: f.team.name })} onclose={() => (found = null)} backdropCloses={false}>
    <p class="muted">{tr("These projects on this computer belong to {team}. Reconnect them to share versions with the team again; their history is kept.", { team: f.team.name })}</p>
    <ul class="found">
      {#each f.projects as p (p.root)}
        <li><label><input type="checkbox" bind:checked={p.on} />
          <span><strong>{p.name}</strong><span class="faint small mono">{p.root}</span></span></label></li>
      {/each}
    </ul>
    {#snippet footer()}
      <button onclick={() => (found = null)}>{tr("Not now")}</button>
      <button class="primary" disabled={reconnecting || !f.projects.some((p) => p.on)} onclick={reconnect}>
        {reconnecting ? tr("Reconnecting…") : tr("Reconnect selected")}</button>
    {/snippet}
  </Modal>
{/if}

{#if settingsFor}
  {@const t = overview.teams.find((x) => x.id === settingsFor!.id) ?? settingsFor}
  <TeamSettings team={t} author={overview.author} {reload} onclose={() => (settingsFor = null)} />
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
  .team-row { display: flex; align-items: center; }
  .team-row .item { flex: 1; min-width: 0; }
  .gear { flex: none; border: none; background: transparent; color: var(--faint); padding: 4px 8px; border-radius: 6px; }
  .gear:hover { color: var(--text); background: #33363d; }
  .check { width: 14px; color: var(--accent); }
  .tname { flex: 1; }
  .small { font-size: 12px; }
  .who {
    width: 100%; margin-top: 6px; padding: 5px 10px; font-size: 12.5px; text-align: left;
    background: var(--warn-bg); color: var(--warn); border: 1px solid #5a4623; border-radius: 8px;
  }
  .sep { height: 1px; background: var(--line); margin: 6px 0; }
  .found { list-style: none; padding: 0; margin: 12px 0 0; display: flex; flex-direction: column; gap: 8px; }
  .found label { display: flex; gap: 10px; align-items: flex-start; margin: 0; color: var(--text); font-size: 14px; }
  .found input { width: auto; margin-top: 3px; }
  .found label > span { flex: 1; min-width: 0; }
  .found .mono { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
</style>
