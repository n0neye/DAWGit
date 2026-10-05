<script lang="ts">
  import { t, tn } from "./i18n.svelte";
  import Modal from "./Modal.svelte";

  // Going to another version replaces the files: uncommitted changes are
  // committed first (described here), or discarded. On an older version
  // they can't be committed here: a branch or the latest version first.
  let { changes, latest, older, team, message = $bindable(), oncommit, ondiscard, onclose }: {
    changes: number;
    latest: boolean; // going back to the latest version (else to an older one)
    older: boolean; // on an older version now
    team: boolean;
    message: string;
    oncommit: () => void;
    ondiscard: () => void;
    onclose: () => void;
  } = $props();
</script>

<Modal title={latest ? t("Back to the latest version") : t("Go to an older version")} {onclose}>
  <p>{tn(changes, "You have {n} uncommitted change.", "You have {n} uncommitted changes.")}
    {latest ? t("Going back replaces the files in the project folder.") : t("Going to another version replaces the files in the project folder.")}</p>
  {#if !older}
    <label for="lm">{t("Commit them first as")}</label>
    <input id="lm" bind:value={message} placeholder={t("What did you change?")} />
  {:else}
    <p class="muted">{team ? t("Changes made on an older version can be kept by starting a new branch from here first.")
      : t("Changes made on an older version can be kept by starting a new branch from here or making it the latest version first.")}</p>
  {/if}
  {#snippet footer()}
    <button onclick={onclose}>{t("Cancel")}</button>
    <button class="danger" onclick={ondiscard}>{t("Discard changes")}</button>
    {#if !older}
      <button class="primary" disabled={!message.trim()} onclick={oncommit}>
        {team ? t("Commit & share, then go") : t("Commit, then go")}
      </button>
    {/if}
  {/snippet}
</Modal>
