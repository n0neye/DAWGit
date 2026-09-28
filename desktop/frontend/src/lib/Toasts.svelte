<script lang="ts">
  import { toasts, dismiss } from "./notify.svelte";
</script>

<div class="toasts">
  {#each toasts as t (t.id)}
    <button class="toast {t.kind}" onclick={() => dismiss(t.id)}>{t.text}</button>
  {/each}
</div>

<style>
  .toasts {
    position: fixed; right: 18px; bottom: 18px; z-index: 60;
    display: flex; flex-direction: column; gap: 8px; max-width: 380px;
  }
  .toast {
    text-align: left; white-space: pre-line; padding: 10px 14px; border-radius: 8px;
    background: var(--panel-2); border: 1px solid var(--line); box-shadow: 0 8px 24px rgba(0, 0, 0, .4);
  }
  .ok { border-left: 3px solid var(--accent); }
  .warn { border-left: 3px solid var(--warn); }
  .error { border-left: 3px solid var(--danger); }
  .info { border-left: 3px solid var(--mod); }
</style>
