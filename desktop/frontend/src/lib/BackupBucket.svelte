<script lang="ts">
  import { t } from "./i18n.svelte";
  import { api, errorText } from "./api";
  import Modal from "./Modal.svelte";
  import StorageFields, { type StorageValue } from "./StorageFields.svelte";

  // Back the team up into S3-compatible storage: another bucket (another
  // Cloudflare account, Backblaze B2, Wasabi, a NAS running MinIO…), or a
  // folder of one. Not the team's own: losing that is what a backup is for.
  let { teamId, ondone, onclose }: { teamId: string; ondone: () => void; onclose: () => void } = $props();

  let storage = $state<StorageValue>({ endpoint: "", bucket: "", folder: "dawgit-backup", region: "", accessKey: "", secretKey: "" });
  let busy = $state(false);
  let error = $state("");

  // Editing the storage in use: its fields as they are.
  $effect(() => {
    api.BackupStorage(teamId).then((s) => {
      if (s) storage = s;
    }).catch(() => {});
  });

  let ready = $derived(!!(storage.endpoint.trim() && storage.bucket.trim() && storage.accessKey.trim() && storage.secretKey.trim()));

  async function use() {
    busy = true;
    error = "";
    try {
      const problem = await api.SetBackupStorage(teamId, storage);
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
  <StorageFields bind:value={storage} id="b" />
  <p class="faint small">{t("The keys need to read, write and list this bucket. They stay on this computer, sealed like the team's.")}</p>
  {#if error}<p class="error small">{error}</p>{/if}
  {#snippet footer()}
    <button onclick={onclose} disabled={busy}>{t("Cancel")}</button>
    <button class="primary" onclick={use} disabled={busy || !ready}>{busy ? t("Checking…") : t("Check & back up there")}</button>
  {/snippet}
</Modal>

<style>
  .small { font-size: 12.5px; }
  .error { color: var(--danger); user-select: text; }
</style>
