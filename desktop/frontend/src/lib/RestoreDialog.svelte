<script lang="ts">
  import { t, tn, language } from "./i18n.svelte";
  import { Events } from "@wailsio/runtime";
  import { api, errorText, formatBytes } from "./api";
  import Modal from "./Modal.svelte";
  import StorageFields, { type StorageValue } from "./StorageFields.svelte";
  import { toast } from "./notify.svelte";

  // Bring back from a backup what the team's storage lacks: deleted
  // projects, lost files, or a whole team (into a new, empty bucket the team
  // was set up on). Only adds; never changes or deletes anything. The backup:
  // this computer's, a folder, or a bucket (a teammate's backup; its keys
  // are used for this restore only).
  let { teamId, hasBackup, onclose, ondone }: {
    teamId: string;
    hasBackup: boolean; // this computer backs the team up (the default source)
    onclose: () => void;
    ondone: () => void;
  } = $props();

  type Plan = Awaited<ReturnType<typeof api.RestorePlan>>;
  let folder = $state(""); // "" = this computer's backup
  let fromBucket = $state(false); // the bucket below instead
  let storage = $state<StorageValue>({ endpoint: "", bucket: "", folder: "dawgit-backup", region: "", accessKey: "", secretKey: "" });
  let bucketReady = $derived(!!(storage.endpoint.trim() && storage.bucket.trim() && storage.accessKey.trim() && storage.secretKey.trim()));
  const source = () => ({ folder: fromBucket ? "" : folder, storage: fromBucket ? storage : null });
  let run = $state("");
  let plan = $state<Plan>(null);
  let loading = $state(false);
  let error = $state("");
  let busy = $state(false);
  let done = $state(0);
  let total = $state(0);

  async function load() {
    loading = true;
    error = "";
    try {
      plan = await api.RestorePlan(teamId, source(), run);
    } catch (e) {
      plan = null;
      error = errorText(e);
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    if (hasBackup) load();
  });

  $effect(() => Events.On("restore", (ev: { data: { teamId: string; done: number; total: number } }) => {
    if (ev.data.teamId === teamId) ({ done, total } = ev.data);
  }));

  async function chooseFolder() {
    const dir = await api.ChooseFolder(t("Choose the backup's folder")).catch(() => "");
    if (!dir) return;
    folder = dir;
    fromBucket = false;
    run = "";
    load();
  }

  async function restore() {
    busy = true;
    error = "";
    try {
      const n = await api.Restore(teamId, source(), run);
      toast(tn(n, "Restored {n} file", "Restored {n} files"), "ok");
      ondone();
    } catch (e) {
      error = errorText(e);
    } finally {
      busy = false;
    }
  }

  // Runs are named by their time in UTC: 20261004-153000.
  function runTime(r: string): string {
    const m = /^(\d{4})(\d{2})(\d{2})-(\d{2})(\d{2})(\d{2})$/.exec(r);
    if (!m) return r;
    const d = new Date(Date.UTC(+m[1], +m[2] - 1, +m[3], +m[4], +m[5], +m[6]));
    return d.toLocaleString(language(), { year: "numeric", month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" });
  }
</script>

<Modal title={t("Restore from backup")} {onclose} width={520} backdropCloses={!busy}>
  <p class="muted small">{t("Brings back what the team's storage lacks: deleted projects, lost files, or the whole team into a new, empty bucket. It only adds; nothing in the team's storage is changed or deleted.")}</p>

  <div class="row src">
    <span class="label">{t("Backup")}</span>
    <span class="where" title={plan?.where ?? folder}>{plan?.where ?? (fromBucket ? t("A bucket") : folder || (hasBackup ? "…" : t("Choose the backup's folder or bucket")))}</span>
    <button class="ghost" disabled={busy} onclick={chooseFolder}>{t("Choose folder…")}</button>
    <button class="ghost" class:on={fromBucket} disabled={busy}
      onclick={() => { fromBucket = !fromBucket; plan = null; error = ""; run = ""; if (!fromBucket && (folder || hasBackup)) load(); }}>{t("Bucket…")}</button>
  </div>

  {#if fromBucket}
    <div class="bucket">
      <StorageFields bind:value={storage} id="rb" />
      <p class="faint small">{t("Where the backup is: the keys only need to read that bucket. They're used for this restore only, not kept.")}</p>
      <button disabled={busy || loading || !bucketReady} onclick={() => { run = ""; load(); }}>{loading ? t("Checking…") : t("Look in this bucket")}</button>
    </div>
  {/if}

  {#if plan}
    {#if plan.runs.length}
      <div class="row src">
        <label class="label" for="r-run">{t("As of")}</label>
        <select id="r-run" bind:value={run} disabled={busy} onchange={load}>
          <option value="">{t("The latest backup")}</option>
          {#each plan.runs as r}<option value={r}>{runTime(r)}</option>{/each}
        </select>
      </div>
    {/if}

    {#if loading}
      <p class="faint small">{t("Comparing with the team's storage…")}</p>
    {:else if !plan.files}
      <p class="ok">✓ {t("The team's storage has everything in this backup. Nothing to restore.")}</p>
    {:else}
      <div class="plan">
        {#if plan.projects.length}
          <p class="small">{tn(plan.projects.length, "{n} project comes back:", "{n} projects come back:")}</p>
          <ul>
            {#each plan.projects as p}
              <li>{p.name} <span class="faint">· {tn(p.versions, "{n} version", "{n} versions")}</span></li>
            {/each}
          </ul>
        {/if}
        {#if plan.branches}
          <p class="small">{tn(plan.branches, "{n} branch comes back in projects the team still has.", "{n} branches come back in projects the team still has.")}</p>
        {/if}
        <p class="faint small">{tn(plan.files, "{n} file to copy ({size}).", "{n} files to copy ({size}).", { size: formatBytes(plan.bytes) })}
          {#if plan.team}{t("Backup of “{team}”.", { team: plan.team })}{/if}</p>
      </div>
    {/if}
  {/if}

  {#if busy}
    <div class="bar"><div style="width: {total ? Math.round((done / total) * 100) : 0}%"></div></div>
    <p class="faint small">{t("Restoring… {done} of {total}", { done: formatBytes(done), total: formatBytes(total || plan?.bytes || 0) })}</p>
  {/if}
  {#if error}<p class="error small">{error}</p>{/if}

  {#snippet footer()}
    <button onclick={onclose} disabled={busy}>{t("Close")}</button>
    <button class="primary" disabled={busy || loading || !plan?.files} onclick={restore}>{busy ? t("Restoring…") : t("Restore")}</button>
  {/snippet}
</Modal>

<style>
  .small { font-size: 12.5px; }
  .src { gap: 10px; align-items: center; margin: 10px 0; }
  .label { flex: none; width: 70px; font-size: 13px; color: var(--muted); margin: 0; }
  .where { flex: 1; min-width: 0; font-family: var(--mono, monospace); font-size: 12.5px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; user-select: text; }
  select { flex: 1; background: var(--panel); color: var(--text); border: 1px solid var(--line); border-radius: 6px; padding: 5px 8px; font: inherit; font-size: 13px; }
  .plan { margin: 12px 0; padding: 10px 12px; border-radius: 8px; background: var(--panel); }
  .plan p { margin: 0 0 6px; }
  .plan ul { margin: 0 0 8px; padding-left: 18px; font-size: 13px; }
  .ok { color: var(--accent); }
  .bar { height: 4px; border-radius: 2px; background: var(--panel); overflow: hidden; margin: 10px 0 4px; }
  .bar div { height: 100%; background: var(--accent); transition: width .3s; }
  .error { color: var(--danger); user-select: text; }
  .bucket { margin: 4px 0 12px; padding: 4px 12px 12px; border-radius: 8px; background: var(--panel); }
  .bucket p { margin: 0 0 8px; }
  button.on { color: var(--accent); }
</style>
