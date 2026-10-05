<script lang="ts">
  import { t } from "./i18n.svelte";
  import Modal from "./Modal.svelte";

  // DAWGit is about to change the project's files while its tool has it
  // open: close it first, then go on. set: the Live set open ("" when Live
  // can't tell); tool: the project's tool ("" counts as Live).
  let { tool, set, oncontinue, onclose }: {
    tool: string; set: string; oncontinue: () => void; onclose: () => void;
  } = $props();
  let live = $derived(tool === "Ableton Live" || !tool);
</script>

<Modal title={live ? (set ? t("“{set}” is open in Live", { set }) : t("Ableton Live is running"))
  : t("{tool} has this project open", { tool: t(tool) })} {onclose}>
  {#if live}
    <p>{t("DAWGit is about to change files in this project. Live keeps the open set in memory and would overwrite the changes the next time you save it.")}</p>
    <p class="muted">{t("Save and close the set in Live first — you can leave Live open with another set.")}
      {set ? t("Live only shows the set's name, so a set with the same name from another project counts too.")
        : t("DAWGit cannot tell which set Live has open: close Live to go on.")}</p>
  {:else}
    <p>{t("DAWGit is about to change files in this project. {tool} may hold some of them open, or write over the changes.", { tool: t(tool) })}</p>
    <p class="muted">{t("Save your work and close the project in {tool} first.", { tool: t(tool) })}</p>
  {/if}
  {#snippet footer()}
    <button onclick={onclose}>{t("Cancel")}</button>
    <button class="primary" onclick={oncontinue}>{t("I closed it — continue")}</button>
  {/snippet}
</Modal>
