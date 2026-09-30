<script lang="ts">
  import { untrack } from "svelte";
  import { api, errorText, type TeamSummary } from "./api";
  import { toast } from "./notify.svelte";
  import Modal from "./Modal.svelte";
  import CodeBox from "./CodeBox.svelte";

  // One team's settings: its name, the connection code for teammates, how
  // this computer reaches it (storage keys, or a server's address), and
  // disconnecting.
  let { team, reload, onclose }: {
    team: TeamSummary;
    reload: () => Promise<void>;
    onclose: () => void;
  } = $props();

  type Conn = { storage: boolean; address: string; token: string;
    settings: { endpoint: string; bucket: string; folder: string; region: string; accessKey: string; secretKey: string } };

  let name = $state(untrack(() => team.name));
  let renaming = $state(false);
  let code = $state("");
  let saved = $state<Conn | null>(null); // as stored
  let conn = $state<Conn | null>(null); // being edited
  let showSecret = $state(false);
  let saving = $state(false);
  let connError = $state("");
  let confirmDisconnect = $state(false);

  $effect(() => {
    const id = team.id;
    api.TeamConnectionSettings(id).then((c) => {
      saved = c as Conn;
      conn = structuredClone($state.snapshot(c)) as Conn;
    }).catch((e) => (connError = errorText(e)));
    if (team.isStorage) api.TeamConnectionCode(id).then((c) => (code = c)).catch(() => (code = ""));
  });

  let changed = $derived(!!conn && !!saved && JSON.stringify(conn) !== JSON.stringify(saved));

  async function renameForEveryone() {
    renaming = true;
    try {
      await api.RenameTeamForEveryone(team.id, name);
      await reload();
      toast(`Renamed to ${name.trim()} for everyone`, "ok");
    } catch (e) {
      toast(errorText(e), "error", 9000);
    } finally {
      renaming = false;
    }
  }

  async function renameHere() {
    try {
      await api.RenameTeam(team.id, name);
      await reload();
      toast("Renamed on this computer", "ok");
    } catch (e) {
      toast(errorText(e), "error");
    }
  }

  async function saveConnection() {
    if (!conn) return;
    saving = true;
    connError = "";
    try {
      await api.UpdateTeamConnection(team.id, conn);
      saved = structuredClone($state.snapshot(conn)) as Conn;
      if (team.isStorage) code = await api.TeamConnectionCode(team.id);
      await reload();
      toast("Connection checked and saved", "ok");
    } catch (e) {
      connError = errorText(e);
    } finally {
      saving = false;
    }
  }

  async function disconnect() {
    try {
      await api.RemoveTeam(team.id);
      await reload();
      toast(`Disconnected from ${team.name}. Project folders were left on disk.`, "info", 7000);
      onclose();
    } catch (e) {
      toast(errorText(e), "error");
    }
  }
</script>

<Modal title="{team.name} settings" {onclose} width={620}>
  <section>
    <h3>Name</h3>
    <input bind:value={name} aria-label="Team name" />
    {#if name !== team.name}
      <div class="row btns">
        {#if name.trim()}
          <button class="primary" disabled={renaming} onclick={renameForEveryone}>{renaming ? "Renaming…" : "Rename for everyone"}</button>
          <button disabled={renaming} onclick={renameHere}>Only on this computer</button>
        {:else}
          <button onclick={renameHere}>Use the team's name</button>
        {/if}
        <button class="ghost" onclick={() => (name = team.name)}>Cancel</button>
      </div>
    {/if}
  </section>

  {#if team.isStorage}
    <section>
      <h3>Invite teammates</h3>
      <p class="faint small">Send this connection code privately; they choose <em>Join a team</em> and paste it.
        It contains the storage key.</p>
      {#if code}<CodeBox {code} />{/if}
    </section>
  {/if}

  <section>
    <h3>Connection</h3>
    {#if conn?.storage}
      <p class="faint small">The bucket and key this computer uses. Change them after making a new key in Cloudflare
        (then send teammates the new code).</p>
      <div class="grid">
        <label for="t-ep">Endpoint</label>
        <input id="t-ep" bind:value={conn.settings.endpoint} spellcheck="false" />
        <label for="t-b">Bucket</label>
        <input id="t-b" bind:value={conn.settings.bucket} spellcheck="false" />
        <label for="t-ak">Access Key ID</label>
        <input id="t-ak" bind:value={conn.settings.accessKey} spellcheck="false" autocomplete="off" />
        <label for="t-sk">Secret Access Key</label>
        <div class="row secret">
          <input id="t-sk" type={showSecret ? "text" : "password"} bind:value={conn.settings.secretKey} autocomplete="off" />
          <button class="ghost" onclick={() => (showSecret = !showSecret)}>{showSecret ? "Hide" : "Show"}</button>
        </div>
        <label for="t-f">Folder</label>
        <input id="t-f" bind:value={conn.settings.folder} spellcheck="false" />
        <label for="t-r">Region</label>
        <input id="t-r" bind:value={conn.settings.region} spellcheck="false" />
      </div>
    {:else if conn}
      <p class="faint small">This team uses a team server.</p>
      <div class="grid">
        <label for="t-a">Server address</label>
        <input id="t-a" bind:value={conn.address} spellcheck="false" />
        <label for="t-t">Access token</label>
        <div class="row secret">
          <input id="t-t" type={showSecret ? "text" : "password"} bind:value={conn.token} autocomplete="off" />
          <button class="ghost" onclick={() => (showSecret = !showSecret)}>{showSecret ? "Hide" : "Show"}</button>
        </div>
      </div>
    {/if}
    {#if connError}<p class="error small">{connError}</p>{/if}
    {#if changed}
      <div class="row btns">
        <button class="primary" disabled={saving} onclick={saveConnection}>{saving ? "Checking…" : "Check & save"}</button>
        <button class="ghost" disabled={saving} onclick={() => { conn = structuredClone($state.snapshot(saved)) as Conn; connError = ""; }}>Cancel</button>
      </div>
    {/if}
  </section>

  {#snippet footer()}
    {#if confirmDisconnect}
      <span class="small confirm">Forget this team and its key on this computer? Project folders stay.</span>
      <button onclick={() => (confirmDisconnect = false)}>Keep</button>
      <button class="danger" onclick={disconnect}>Disconnect</button>
    {:else}
      <button class="ghost danger-text" onclick={() => (confirmDisconnect = true)}>Disconnect…</button>
      <span class="spacer"></span>
      <button onclick={onclose}>Close</button>
    {/if}
  {/snippet}
</Modal>

<style>
  section { margin-bottom: 18px; }
  h3 { font-size: 12px; text-transform: uppercase; letter-spacing: .06em; color: var(--muted); margin: 0 0 8px; }
  .small { font-size: 12.5px; }
  section > p { margin: 0 0 8px; }
  .btns { gap: 6px; margin-top: 8px; }
  .btns button { padding: 5px 10px; font-size: 13px; }
  .grid { display: grid; grid-template-columns: auto 1fr; gap: 8px 12px; align-items: center; }
  .grid label { margin: 0; font-size: 13px; }
  .secret { gap: 6px; }
  .secret input { flex: 1; min-width: 0; }
  .error { color: var(--danger); user-select: text; }
  .danger-text { color: var(--danger); }
  .confirm { flex: 1; color: var(--muted); align-self: center; }
</style>
