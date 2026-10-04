<script lang="ts">
  import { t, tn } from "./i18n.svelte";
  import { api, errorText } from "./api";
  import Modal from "./Modal.svelte";

  // Before projects leave DAWGit (unlinked, deleted from the team, the team
  // left): samples that came from teammates' computers are only in each
  // project's hidden .dawgit folder, gone if it is deleted. Offered: put
  // them in the project (Samples/Imported) and point the sets there. Sets
  // are rewritten, so not while one is open in Live. When there are none,
  // it goes straight on.
  let { roots, onproceed, oncancel }: { roots: string[]; onproceed: () => void; oncancel: () => void } = $props();

  let count = $state<number | null>(null);
  let busy = $state(false);
  let error = $state("");

  $effect(() => {
    Promise.all(roots.filter(Boolean).map((r) => api.SampleSpots(r).catch(() => [])))
      .then((lists) => {
        count = lists.reduce((n, l) => n + (l ?? []).filter((s) => s.kept).length, 0);
        if (count === 0) onproceed();
      });
  });

  async function bringIn() {
    busy = true;
    error = "";
    try {
      for (const r of roots.filter(Boolean)) {
        const res = await api.BringSamplesIn(r, false, true, false);
        if (res?.liveRunning) {
          error = res.openSet
            ? t("“{set}” is open in Ableton Live. Save and close it in Live, then try again.", { set: res.openSet })
            : t("A set of this project is open in Ableton Live. Save and close it in Live, then try again.");
          return;
        }
      }
      onproceed();
    } catch (e) {
      error = errorText(e);
    } finally {
      busy = false;
    }
  }
</script>

{#if count}
  <Modal title={t("Put teammates' samples into the project?")} onclose={oncancel}>
    <p>{tn(count, "{n} sample the sets use came from a teammate's computer and is only in DAWGit's hidden .dawgit folder. Delete that folder and the sets lose it.", "{n} samples the sets use came from teammates' computers and are only in DAWGit's hidden .dawgit folder. Delete that folder and the sets lose them.")}</p>
    <p>{t("DAWGit can copy them into the project's Samples/Imported folder and point the sets there, so the project keeps working without DAWGit.")}</p>
    <p class="muted">{t("Save and close the project's sets in Ableton Live first.")}</p>
    {#if error}<p class="error">{error}</p>{/if}
    {#snippet footer()}
      <button onclick={oncancel} disabled={busy}>{t("Cancel")}</button>
      <button onclick={onproceed} disabled={busy}>{t("Skip")}</button>
      <button class="primary" onclick={bringIn} disabled={busy}>{busy ? t("Copying…") : t("Put them in and continue")}</button>
    {/snippet}
  </Modal>
{/if}

<style>
  .error { color: var(--warn); }
</style>
