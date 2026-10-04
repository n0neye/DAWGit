<script lang="ts">
  import { t } from "./i18n.svelte";
  import { api, errorText } from "./api";
  import Modal from "./Modal.svelte";

  // Back the team up into S3-compatible storage: another bucket (another
  // Cloudflare account, Backblaze B2, Wasabi, a NAS running MinIO…), or a
  // folder of one. Not the team's own: losing that is what a backup is for.
  let { teamId, ondone, onclose }: { teamId: string; ondone: () => void; onclose: () => void } = $props();

  let endpoint = $state("");
  let bucket = $state("");
  let folder = $state("dawgit-backup");
  let region = $state("");
  let accessKey = $state("");
  let secretKey = $state("");
  let showSecret = $state(false);
  let busy = $state(false);
  let error = $state("");

  // Editing the storage in use: its fields as they are.
  $effect(() => {
    api.BackupStorage(teamId).then((s) => {
      if (!s) return;
      ({ endpoint, bucket, folder, region, accessKey, secretKey } = s);
    }).catch(() => {});
  });

  let ready = $derived(!!(endpoint.trim() && bucket.trim() && accessKey.trim() && secretKey.trim()));

  async function use() {
    busy = true;
    error = "";
    try {
      const problem = await api.SetBackupStorage(teamId, { endpoint, bucket, folder, region, accessKey, secretKey });
      if (problem === "same-storage") error = t("That is where the team keeps its work. A backup needs to be somewhere else: another bucket, or another account.");
      else if (problem === "other-team") error = t("That bucket folder holds another team's backup. Choose another folder.");
      else if (problem === "not-empty") error = t("That bucket folder has other things in it. Choose an empty folder (or a new name), or this team's earlier backup.");
      else ondone();
    } catch (e) {
      error = errorText(e);
    } finally {
      busy = false;
    }
  }
</script>

<Modal title={t("Back up to another bucket")} {onclose}>
  <p class="muted small">{t("Any S3-compatible storage: a bucket in another Cloudflare account, Backblaze B2, Wasabi, Amazon S3, or a NAS running MinIO. Best with keys of their own, so a leaked or lost team key can't reach the backup.")}</p>
  <div class="grid">
    <label for="b-ep">{t("Endpoint")}</label>
    <input id="b-ep" bind:value={endpoint} spellcheck="false" autocomplete="off" placeholder="https://<account id>.r2.cloudflarestorage.com" />
    <label for="b-b">{t("Bucket")}</label>
    <input id="b-b" bind:value={bucket} spellcheck="false" />
    <label for="b-f">{t("Folder")}</label>
    <input id="b-f" bind:value={folder} spellcheck="false" />
    <label for="b-r">{t("Region")}</label>
    <input id="b-r" bind:value={region} spellcheck="false" placeholder="auto" />
    <label for="b-ak">{t("Access Key ID")}</label>
    <input id="b-ak" bind:value={accessKey} spellcheck="false" autocomplete="off" />
    <label for="b-sk">{t("Secret Access Key")}</label>
    <div class="row secret">
      <input id="b-sk" type={showSecret ? "text" : "password"} bind:value={secretKey} autocomplete="off" />
      <button class="ghost" onclick={() => (showSecret = !showSecret)}>{showSecret ? t("Hide") : t("Show")}</button>
    </div>
  </div>
  <p class="faint small">{t("The keys need to read, write and list this bucket. They stay on this computer, sealed like the team's.")}</p>
  {#if error}<p class="error small">{error}</p>{/if}
  {#snippet footer()}
    <button onclick={onclose} disabled={busy}>{t("Cancel")}</button>
    <button class="primary" onclick={use} disabled={busy || !ready}>{busy ? t("Checking…") : t("Check & back up there")}</button>
  {/snippet}
</Modal>

<style>
  .small { font-size: 12.5px; }
  .grid { display: grid; grid-template-columns: auto 1fr; gap: 8px 12px; align-items: center; margin: 10px 0; }
  .grid label { margin: 0; font-size: 13px; }
  .secret { gap: 6px; }
  .secret input { flex: 1; min-width: 0; }
  .error { color: var(--danger); user-select: text; }
</style>
