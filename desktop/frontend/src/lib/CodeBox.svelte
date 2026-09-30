<script lang="ts">
  // A connection code with a copy button.
  let { code }: { code: string } = $props();
  let copied = $state(false);

  async function copy() {
    try {
      await navigator.clipboard.writeText(code);
      copied = true;
      setTimeout(() => (copied = false), 2000);
    } catch {
      copied = false;
    }
  }
</script>

<div class="code">
  <input class="mono" readonly value={code} aria-label="Connection code" onfocus={(e) => e.currentTarget.select()} />
  <button class:primary={!copied} onclick={copy}>{copied ? "✓ Copied" : "Copy"}</button>
</div>

<style>
  .code { display: flex; gap: 8px; margin: 0 0 10px; }
  input { flex: 1; min-width: 0; font-size: 12px; color: var(--muted); }
  button { flex: none; min-width: 84px; }
</style>
