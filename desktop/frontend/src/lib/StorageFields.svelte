<script lang="ts" module>
  export type StorageValue = { endpoint: string; bucket: string; folder: string; region: string; accessKey: string; secretKey: string };
</script>

<script lang="ts">
  import { t } from "./i18n.svelte";

  // Where S3-compatible storage is and its keys: a backup bucket, to back
  // up into or restore from.
  let { value = $bindable(), id = "s" }: { value: StorageValue; id?: string } = $props();
  let showSecret = $state(false);
</script>

<div class="grid">
  <label for="{id}-ep">{t("Endpoint")}</label>
  <input id="{id}-ep" bind:value={value.endpoint} spellcheck="false" autocomplete="off" placeholder="https://<account id>.r2.cloudflarestorage.com" />
  <label for="{id}-b">{t("Bucket")}</label>
  <input id="{id}-b" bind:value={value.bucket} spellcheck="false" />
  <label for="{id}-f">{t("Folder")}</label>
  <input id="{id}-f" bind:value={value.folder} spellcheck="false" />
  <label for="{id}-r">{t("Region")}</label>
  <input id="{id}-r" bind:value={value.region} spellcheck="false" placeholder="auto" />
  <label for="{id}-ak">{t("Access Key ID")}</label>
  <input id="{id}-ak" bind:value={value.accessKey} spellcheck="false" autocomplete="off" />
  <label for="{id}-sk">{t("Secret Access Key")}</label>
  <div class="row secret">
    <input id="{id}-sk" type={showSecret ? "text" : "password"} bind:value={value.secretKey} autocomplete="off" />
    <button class="ghost" onclick={() => (showSecret = !showSecret)}>{showSecret ? t("Hide") : t("Show")}</button>
  </div>
</div>

<style>
  .grid { display: grid; grid-template-columns: auto 1fr; gap: 8px 12px; align-items: center; margin: 10px 0; }
  .grid label { margin: 0; font-size: 13px; }
  .secret { gap: 6px; }
  .secret input { flex: 1; min-width: 0; }
</style>
