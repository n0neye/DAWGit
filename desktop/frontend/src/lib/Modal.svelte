<script lang="ts">
  import type { Snippet } from "svelte";

  let { title, onclose, width = 560, backdropCloses = true, children, footer }: {
    title: string;
    onclose: () => void;
    width?: number;
    // false for forms that take a while to fill in: only ✕ closes them.
    backdropCloses?: boolean;
    children: Snippet;
    footer?: Snippet;
  } = $props();

  function onkeydown(e: KeyboardEvent) {
    if (e.key === "Escape" && backdropCloses) onclose();
  }
</script>

<svelte:window {onkeydown} />

<div class="backdrop" role="presentation" onclick={(e) => { if (backdropCloses && e.target === e.currentTarget) onclose(); }}>
  <div class="modal" style:width="{width}px" role="dialog" aria-modal="true" aria-label={title}>
    <header>
      <h2>{title}</h2>
      <button class="ghost close" onclick={onclose} aria-label="Close">✕</button>
    </header>
    <div class="body">{@render children()}</div>
    {#if footer}
      <footer>{@render footer()}</footer>
    {/if}
  </div>
</div>

<style>
  .backdrop {
    position: fixed; inset: 0; background: rgba(8, 9, 11, .62);
    display: flex; align-items: center; justify-content: center; z-index: 50;
  }
  .modal {
    max-width: calc(100vw - 48px); max-height: calc(100vh - 48px);
    background: var(--panel); border: 1px solid var(--line); border-radius: 12px;
    display: flex; flex-direction: column; box-shadow: 0 20px 60px rgba(0, 0, 0, .5);
  }
  header { display: flex; align-items: center; padding: 16px 18px 8px; }
  h2 { margin: 0; font-size: 16px; font-weight: 600; flex: 1; }
  .close { padding: 2px 8px; }
  .body { padding: 6px 18px 12px; overflow: auto; }
  footer {
    display: flex; justify-content: flex-end; gap: 8px; padding: 12px 18px;
    border-top: 1px solid var(--line);
  }
</style>
