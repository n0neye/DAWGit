<script lang="ts">
  import { t, tn } from "./i18n.svelte";
  import Tx from "./Tx.svelte";
  import { untrack } from "svelte";
  import { api, errorText, type TeamSummary } from "./api";
  import { toast } from "./notify.svelte";
  import Modal from "./Modal.svelte";
  import KeptSamples from "./KeptSamples.svelte";
  import CodeBox from "./CodeBox.svelte";
  import IdentityForm from "./IdentityForm.svelte";

  // One team's settings: its name, your name in it, the connection code for
  // teammates, how this computer reaches it (storage keys, or a server's
  // address), and disconnecting.
  let { team, author = "", reload, onclose, roots = [] }: {
    team: TeamSummary;
    roots?: string[]; // its projects on this computer (kept under Local when leaving)
    author?: string; // this computer's name, suggested when you have none here
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
  let keepProjects = $state(true); // move the team's projects to Local
  let fullHistory = $state(false); // and download older versions' files
  let historySize = $state(0); // bytes those weigh
  let leaving = $state(false);
  const mb = (n: number) => (n >= 1 << 30 ? `${(n / (1 << 30)).toFixed(1)} GB` : `${Math.max(1, Math.round(n / (1 << 20)))} MB`);
  let editingMe = $state(false);
  let shareSetup = $state(untrack(() => team.shareSetup));
  let shareError = $state("");
  async function toggleShare(on: boolean) {
    shareError = "";
    try {
      await api.SetShareSetup(team.id, on);
      shareSetup = on;
      await reload();
    } catch (e) {
      shareError = errorText(e);
    }
  }

  // Storage cleanup: files no version of any project uses.
  type Cleanup = { versions: number; stored: number; unused: number; unusedBytes: number; due: number;
    dueBytes: number; deleted: number; deletedBytes: number; nextCleanup: string };
  let cleanup = $state<Cleanup | null>(null);
  let cleaning = $state<"" | "check" | "delete">("");
  let cleanError = $state("");
  const when = (iso: string) => new Date(iso).toLocaleString(undefined, { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" });
  async function clean(remove: boolean) {
    cleaning = remove ? "delete" : "check";
    cleanError = "";
    try {
      cleanup = (await api.CleanUpStorage(team.id, remove)) as Cleanup;
      if (remove) toast(tn(cleanup.deleted, "Deleted {n} unused file ({size})", "Deleted {n} unused files ({size})", { size: mb(cleanup.deletedBytes) }), "ok");
    } catch (e) {
      cleanError = errorText(e);
    } finally {
      cleaning = "";
    }
  }

  async function identitySaved(s: TeamSummary) {
    const renamed = !!team.memberId && s.memberName !== team.memberName;
    editingMe = false;
    await reload();
    toast(t(renamed ? "You're “{name}” in {team} — on all your versions" : "You're “{name}” in {team}", { name: s.memberName, team: s.name }), "ok");
  }

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
      toast(t("Renamed to {name} for everyone", { name: name.trim() }), "ok");
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
      toast(t("Renamed on this computer"), "ok");
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
      toast(t("Connection checked and saved"), "ok");
    } catch (e) {
      connError = errorText(e);
    } finally {
      saving = false;
    }
  }

  // Leaving with the projects kept: teammates' samples go into them first.
  let keeping = $state(false);
  function disconnectAsked() {
    if (keepProjects && roots.length) keeping = true;
    else disconnect();
  }

  async function disconnect() {
    try {
      leaving = true;
      await api.RemoveTeam(team.id, keepProjects, keepProjects && fullHistory);
      await reload();
      toast(t(keepProjects ? "Disconnected from {team}. Its projects are under Local now." : "Disconnected from {team}. Project folders were left on disk.",
        { team: team.name }), "info", 7000);
      onclose();
    } catch (e) {
      toast(errorText(e), "error", 9000);
    } finally {
      leaving = false;
    }
  }
</script>

<Modal title={t("{team} settings", { team: team.name })} {onclose} width={620} backdropCloses={false}>
  <section>
    <h3>{t("Name")}</h3>
    <input bind:value={name} aria-label={t("Team name")} />
    {#if name !== team.name}
      <div class="row btns">
        {#if name.trim()}
          <button class="primary" disabled={renaming} onclick={renameForEveryone}>{renaming ? "Renaming…" : "Rename for everyone"}</button>
          <button disabled={renaming} onclick={renameHere}>{t("Only on this computer")}</button>
        {:else}
          <button onclick={renameHere}>{t("Use the team's name")}</button>
        {/if}
        <button class="ghost" onclick={() => (name = team.name)}>{t("Cancel")}</button>
      </div>
    {/if}
  </section>

  <section>
    <h3>{t("Your name in this team")}</h3>
    {#if team.memberId && !editingMe}
      <div class="row me">
        <span class="myname">{team.memberName}</span>
        <button onclick={() => (editingMe = true)}>{t("Change…")}</button>
      </div>
      <p class="faint small">{t("Shown next to the versions you commit, for everyone in the team.")}</p>
    {:else}
      {#key team.memberName}
        <IdentityForm {team} suggested={author} submitLabel="Save" onsaved={identitySaved} />
      {/key}
      {#if team.memberId}<button class="ghost small" onclick={() => (editingMe = false)}>{t("Cancel")}</button>{/if}
    {/if}
  </section>

  {#if team.canShareSetup}
    <section>
      <h3>{t("Your setup")}</h3>
      <label class="share"><input type="checkbox" checked={shareSetup} onchange={(e) => toggleShare(e.currentTarget.checked)} />
        {t("Share my setup with the team")}</label>
      <p class="faint small">{t("Which versions of your creative apps you have (such as Ableton Live) and the names and versions of your plugins and packs. No files or folders. A project check can then tell whether teammates can open a project.")} {t("Turning it off removes it from the team's storage.")}</p>
      {#if shareError}<p class="error">{shareError}</p>{/if}
    </section>
  {/if}

  {#if team.isStorage}
    <section>
      <h3>{t("Invite teammates")}</h3>
      <p class="faint small"><Tx text={t("Send this connection code privately; they choose {join} and paste it. It contains the storage key.")} em={{ join: t("Join a team") }} /></p>
      {#if code}<CodeBox {code} />{/if}
    </section>
  {/if}

  <section>
    <h3>{t("Connection")}</h3>
    {#if conn?.storage}
      <p class="faint small">{t("The bucket and key this computer uses. Change them after making a new key in Cloudflare (then send teammates the new code).")}</p>
      <div class="grid">
        <label for="t-ep">{t("Endpoint")}</label>
        <input id="t-ep" bind:value={conn.settings.endpoint} spellcheck="false" />
        <label for="t-b">{t("Bucket")}</label>
        <input id="t-b" bind:value={conn.settings.bucket} spellcheck="false" />
        <label for="t-ak">{t("Access Key ID")}</label>
        <input id="t-ak" bind:value={conn.settings.accessKey} spellcheck="false" autocomplete="off" />
        <label for="t-sk">{t("Secret Access Key")}</label>
        <div class="row secret">
          <input id="t-sk" type={showSecret ? "text" : "password"} bind:value={conn.settings.secretKey} autocomplete="off" />
          <button class="ghost" onclick={() => (showSecret = !showSecret)}>{showSecret ? "Hide" : "Show"}</button>
        </div>
        <label for="t-f">{t("Folder")}</label>
        <input id="t-f" bind:value={conn.settings.folder} spellcheck="false" />
        <label for="t-r">{t("Region")}</label>
        <input id="t-r" bind:value={conn.settings.region} spellcheck="false" />
      </div>
    {:else if conn}
      <p class="faint small">{t("This team uses a team server.")}</p>
      <div class="grid">
        <label for="t-a">{t("Server address")}</label>
        <input id="t-a" bind:value={conn.address} spellcheck="false" />
        <label for="t-t">{t("Access token")}</label>
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
        <button class="ghost" disabled={saving} onclick={() => { conn = structuredClone($state.snapshot(saved)) as Conn; connError = ""; }}>{t("Cancel")}</button>
      </div>
    {/if}
  </section>

  {#if team.isStorage}
    <section>
      <h3>{t("Storage cleanup")}</h3>
      <p class="faint small">{t("Files no version of any project uses — left by deleted projects, or by uploads that stopped — still take space in the bucket. DAWGit deletes them only once they've been unused for a day and are a week old, so it never takes a file a teammate is sharing right now.")}</p>
      {#if cleanup}
        {@const c = cleanup}
        {#if c.deleted}
          <p class="small">{tn(c.deleted, "Deleted {n} file ({size}).", "Deleted {n} files ({size}).", { size: mb(c.deletedBytes) })}</p>
        {/if}
        {#if c.unused - c.deleted === 0}
          <p class="small">{t(c.deleted ? "Nothing else is unused: {files} files, used by {versions} versions." : "Nothing unused: {files} files, used by {versions} versions.",
            { files: c.stored, versions: c.versions })}</p>
        {:else if c.due && !c.deleted}
          <p class="small">{tn(c.unused, "{n} unused file ({size}); {due} ({dueSize}) can be deleted now.", "{n} unused files ({size}); {due} ({dueSize}) can be deleted now.",
            { size: mb(c.unusedBytes), due: c.due, dueSize: mb(c.dueBytes) })}</p>
        {:else}
          <p class="small">{c.nextCleanup
            ? tn(c.unused - c.deleted, "{n} unused file ({size}) can be deleted from {when}: come back and clean up again then.", "{n} unused files ({size}) can be deleted from {when}: come back and clean up again then.",
              { size: mb(c.unusedBytes - c.deletedBytes), when: when(c.nextCleanup) })
            : tn(c.unused - c.deleted, "{n} unused file ({size}) can be deleted later: come back and clean up again then.", "{n} unused files ({size}) can be deleted later: come back and clean up again then.",
              { size: mb(c.unusedBytes - c.deletedBytes) })}</p>
        {/if}
      {/if}
      {#if cleanError}<p class="error small">{cleanError}</p>{/if}
      <div class="row btns">
        <button disabled={!!cleaning} onclick={() => clean(false)}>{cleaning === "check" ? t("Looking…") : t("Find unused files")}</button>
        {#if cleanup?.due && !cleanup.deleted}
          <button class="danger" disabled={!!cleaning} onclick={() => clean(true)}>
            {cleaning === "delete" ? t("Deleting…") : tn(cleanup.due, "Delete {n} file ({size})", "Delete {n} files ({size})", { size: mb(cleanup.dueBytes) })}</button>
        {/if}
      </div>
    </section>
  {/if}

  {#snippet footer()}
    <button class="ghost danger-text" onclick={() => {
      keepProjects = true; fullHistory = false; historySize = 0; confirmDisconnect = true;
      api.HistoryDownloadSize("", team.id).then((n) => (historySize = n)).catch(() => {});
    }}>{t("Disconnect…")}</button>
    <span class="spacer"></span>
    <button onclick={onclose}>{t("Close")}</button>
  {/snippet}
</Modal>

{#if confirmDisconnect}
  <Modal title={t("Disconnect from {team}?", { team: team.name })} onclose={() => (confirmDisconnect = false)}>
    <p>{t("This computer forgets the team and its key. Nothing changes for your teammates, and project folders stay on disk.")}</p>
    <label class="keep">
      <input type="checkbox" bind:checked={keepProjects} />
      <span><Tx text={t("Move this team's projects to {local}")} strong={{ local: t("Local") }} />
        <span class="faint small">{t("Their versions stay and you can keep committing on this computer. Join the team again later to reconnect them.")}</span></span>
    </label>
    {#if keepProjects && historySize > 0}
      <label class="keep sub">
        <input type="checkbox" bind:checked={fullHistory} />
        <span>{t("Also download the files of older versions ({size})", { size: mb(historySize) })}
          <span class="faint small">{t("Without them, older versions that use other samples than today's need the team again to open.")}</span></span>
      </label>
    {/if}
    {#snippet footer()}
      <button onclick={() => (confirmDisconnect = false)} disabled={leaving}>{t("Cancel")}</button>
      <button class="danger" onclick={disconnectAsked} disabled={leaving}>
        {leaving ? (fullHistory ? t("Downloading…") : t("Disconnecting…")) : t("Disconnect")}</button>
    {/snippet}
  </Modal>

{#if keeping}
  <KeptSamples {roots} oncancel={() => (keeping = false)} onproceed={() => { keeping = false; disconnect(); }} />
{/if}
{/if}

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
  .share { display: flex; align-items: center; gap: 8px; color: var(--text); font-size: 14px; }
  .share input { width: auto; }
  .danger-text { color: var(--danger); }
  .me { gap: 10px; align-items: center; margin-bottom: 4px; }
  .me button { padding: 4px 10px; font-size: 13px; }
  .myname { font-weight: 600; }
  .keep { display: flex; gap: 10px; align-items: flex-start; margin: 12px 0 0; color: var(--text); font-size: 14px; }
  .keep input { width: auto; margin-top: 3px; }
  .keep .faint { display: block; margin-top: 2px; }
  .keep.sub { margin-left: 24px; }
</style>
