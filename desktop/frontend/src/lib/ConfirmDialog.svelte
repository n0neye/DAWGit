<script lang="ts">
  import { t } from "./i18n.svelte";
  import Modal from "./Modal.svelte";

  // Asks before doing something: what happens, and a warning when there is
  // one. danger: the button says it can't be undone (red).
  let { title, text, warn = "", confirm, danger = false, onconfirm, onclose }: {
    title: string; text: string; warn?: string; confirm: string; danger?: boolean;
    onconfirm: () => void; onclose: () => void;
  } = $props();
</script>

<Modal {title} {onclose}>
  <p>{text}</p>
  {#if warn}<p class="warn-text">{warn}</p>{/if}
  {#snippet footer()}
    <button onclick={onclose}>{t("Cancel")}</button>
    <button class={danger ? "danger" : "primary"} onclick={onconfirm}>{confirm}</button>
  {/snippet}
</Modal>

<style>
  .warn-text { color: var(--warn); }
</style>
