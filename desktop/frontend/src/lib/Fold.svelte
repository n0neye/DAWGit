<script lang="ts">
  import type { Snippet } from "svelte";

  // A settings section folded to its title and what matters at a glance
  // (summary); the rest opens in place.
  let { title, summary, warn = false, open = $bindable(false), children }: {
    title: string;
    summary?: Snippet;
    warn?: boolean; // the summary needs attention
    open?: boolean;
    children: Snippet;
  } = $props();
</script>

<section class="fold" class:open>
  <button class="head" aria-expanded={open} onclick={() => (open = !open)}>
    <span class="title">{title}</span>
    {#if !open && summary}<span class="summary" class:warn>{@render summary()}</span>{/if}
    <span class="chev" aria-hidden="true">›</span>
  </button>
  {#if open}
    <div class="body">{@render children()}</div>
  {/if}
</section>

<style>
  .fold { margin-bottom: 18px; }
  .head {
    width: 100%; display: flex; align-items: baseline; gap: 10px; padding: 8px 10px; margin: 0;
    background: var(--panel); border: 1px solid transparent; border-radius: 8px; text-align: left; cursor: pointer;
  }
  .head:hover { border-color: var(--border, #333); }
  .open .head { background: none; padding: 0 0 8px; }
  .title { flex: none; font-size: 12px; text-transform: uppercase; letter-spacing: .06em; color: var(--muted); }
  .summary { flex: 1; min-width: 0; font-size: 12.5px; color: var(--text); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .summary.warn { color: var(--warn); }
  .chev { margin-left: auto; color: var(--muted); transition: transform .15s; }
  .open .chev { transform: rotate(90deg); }
</style>
