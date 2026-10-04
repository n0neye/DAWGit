<script lang="ts">
  import { t, tn } from "./i18n.svelte";
  import { Events } from "@wailsio/runtime";
  import { api, ago, errorText, formatBytes } from "./api";
  import Modal from "./Modal.svelte";
  import BackupBucket from "./BackupBucket.svelte";

  // Backing up the whole team (every project, every version) to a folder on
  // a drive or NAS, once a day while DAWGit runs (desktop/backup.go).
  let { teamId }: { teamId: string } = $props();

  type Info = Awaited<ReturnType<typeof api.BackupInfo>>;
  let info = $state<Info>(null);
  let problem = $state(""); // why a chosen folder can't be used
  let error = $state("");
  let confirmStop = $state(false);
  let changing = $state(false); // choosing another place
  let bucketForm = $state(false);

  async function load() {
    try {
      info = await api.BackupInfo(teamId);
    } catch (e) {
      error = errorText(e);
    }
  }

  $effect(() => {
    teamId;
    load();
    return Events.On("backup", (ev: { data: string }) => {
      if (ev.data === teamId) load();
    });
  });

  async function choose() {
    problem = error = "";
    try {
      const dir = await api.ChooseFolder(t("Choose a backup folder"));
      if (!dir) return;
      problem = await api.SetBackupFolder(teamId, dir);
      if (!problem) changing = false;
      await load();
    } catch (e) {
      error = errorText(e);
    }
  }

  async function act(f: () => Promise<unknown>) {
    error = "";
    try {
      await f();
      await load();
    } catch (e) {
      error = errorText(e);
    }
  }

  const others = $derived(info?.others ?? []);
</script>

{#if info?.supported}
  <section>
    <h3>{t("Backup")}</h3>
    {#if !info.folder}
      <p class="faint small">{t("Keep a copy of the whole team — every project, every version — on a drive, a NAS or another bucket. Once a day while DAWGit is open, it copies what's new. Nothing is ever deleted from the backup.")}</p>
    {:else}
      <p class="folder small" title={info.folder}>{info.folder}</p>
      {#if info.running}
        <p class="small">{info.total > 0
          ? t("Backing up… {done} of {total}", { done: formatBytes(info.done), total: formatBytes(info.total) })
          : t("Backing up…")}</p>
      {:else if info.problem === "missing"}
        <p class="small" class:warn={info.failing}>{info.kind === "s3"
          ? t("The backup isn't in that bucket any more (or the bucket can't be found).")
          : t("The backup folder isn't there. Is the drive connected?")}</p>
      {:else if info.problem}
        <p class="small" class:warn={info.failing}>{t("The last backup didn't finish: {error}", { error: info.error })}</p>
      {/if}
      {#if !info.running}
        <p class="faint small">
          {info.lastSuccess
            ? t("Last backup {when} · {size}", { when: ago(info.lastSuccess), size: formatBytes(info.size) })
            : t("No backup yet.")}
          {#if info.paused}· {t("Paused")}{/if}
        </p>
      {/if}
    {/if}
    {#if others.length}
      <p class="faint small">
        {#each others as o, i}{i ? " · " : ""}{o.lastSuccess
          ? t("{name} backs up the team (last {when})", { name: o.name, when: ago(o.lastSuccess) })
          : t("{name} backs up the team (no backup yet)", { name: o.name })}{/each}
      </p>
    {/if}
    {#if problem === "other-team"}
      <p class="error small">{t("That folder holds another team's backup. Choose another one.")}</p>
    {:else if problem === "not-empty"}
      <p class="error small">{t("That folder has other things in it. Choose an empty folder (or make a new one), or this team's earlier backup.")}</p>
    {/if}
    {#if error}<p class="error small">{error}</p>{/if}
    <div class="row btns">
      {#if !info.folder || changing}
        <button class:primary={!info.folder} onclick={choose}>{t("Choose a backup folder…")}</button>
        <button class="ghost" onclick={() => (bucketForm = true)}>{t("Another bucket…")}</button>
        {#if changing}<button class="ghost" onclick={() => { changing = false; problem = ""; }}>{t("Cancel")}</button>{/if}
      {:else}
        <button disabled={info.running} onclick={() => act(() => api.BackUpNow(teamId))}>{t("Back up now")}</button>
        <button class="ghost" onclick={() => act(() => api.PauseBackup(teamId, !info!.paused))}>{info.paused ? t("Resume") : t("Pause")}</button>
        <button class="ghost" disabled={info.running} onclick={() => (changing = true)}>{t("Change…")}</button>
        <button class="ghost" disabled={info.running} onclick={() => (confirmStop = true)}>{t("Stop backing up")}</button>
      {/if}
    </div>
  </section>
{/if}

{#if bucketForm}
  <BackupBucket {teamId} onclose={() => (bucketForm = false)} ondone={() => { bucketForm = false; changing = false; problem = ""; load(); }} />
{/if}

{#if confirmStop}
  <Modal title={t("Stop backing up?")} onclose={() => (confirmStop = false)}>
    <p>{t("The backup stays as it is. DAWGit just stops adding to it.")}</p>
    {#snippet footer()}
      <button onclick={() => (confirmStop = false)}>{t("Cancel")}</button>
      <button class="danger" onclick={() => { confirmStop = false; act(() => api.StopBackup(teamId)); }}>{t("Stop backing up")}</button>
    {/snippet}
  </Modal>
{/if}

<style>
  section { margin-bottom: 18px; }
  h3 { font-size: 12px; text-transform: uppercase; letter-spacing: .06em; color: var(--muted); margin: 0 0 8px; }
  .small { font-size: 12.5px; }
  section > p { margin: 0 0 8px; }
  .folder { font-family: var(--mono, monospace); user-select: text; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .btns { gap: 6px; margin-top: 8px; flex-wrap: wrap; }
  .btns button { padding: 5px 10px; font-size: 13px; }
  .warn { color: var(--warn); }
  .error { color: var(--danger); user-select: text; }
</style>
