<script lang="ts">
  import { t } from "./i18n.svelte";
  import Modal from "./Modal.svelte";

  // Asks for a name or a description before doing something.
  let { title, text, label, value = $bindable(), placeholder = "", confirm, busy = false, onconfirm, onclose }: {
    title: string; text: string; label: string; value: string; placeholder?: string; confirm: string;
    busy?: boolean; onconfirm: () => void; onclose: () => void;
  } = $props();
  const id = `prompt-${Math.random().toString(36).slice(2)}`;
</script>

<Modal {title} {onclose}>
  <p class="muted">{text}</p>
  <label for={id}>{label}</label>
  <input {id} bind:value {placeholder} />
  {#snippet footer()}
    <button onclick={onclose}>{t("Cancel")}</button>
    <button class="primary" disabled={!value?.trim() || busy} onclick={onconfirm}>{confirm}</button>
  {/snippet}
</Modal>
