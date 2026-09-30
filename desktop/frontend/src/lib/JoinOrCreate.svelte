<script lang="ts">
  import type { TeamSummary } from "./api";
  import ConnectForm from "./ConnectForm.svelte";
  import StorageSetup from "./StorageSetup.svelte";

  // Join a team with a connection code, or create one on your own storage.
  let { onconnected }: { onconnected: (t: TeamSummary) => void } = $props();
  let mode = $state<"join" | "create">("join");
  let created = $state(false); // showing the new team's code: no switching back
</script>

{#if !created}
<div class="seg" role="tablist">
  <button role="tab" class:on={mode === "join"} aria-selected={mode === "join"} onclick={() => (mode = "join")}>Join a team</button>
  <button role="tab" class:on={mode === "create"} aria-selected={mode === "create"} onclick={() => (mode = "create")}>Create a team</button>
</div>
{/if}

{#if mode === "join"}
  <p class="muted">Paste the connection code from whoever set up your team.
    <button class="link" onclick={() => (mode = "create")}>Setting it up yourself?</button></p>
  <ConnectForm {onconnected} />
{:else}
  <StorageSetup {onconnected} oncreated={() => (created = true)} />
{/if}

<style>
  .seg { display: flex; margin: 0 0 14px; }
  .seg button { flex: 1; border-radius: 0; padding: 7px 12px; }
  .seg button:first-child { border-radius: 8px 0 0 8px; }
  .seg button:last-child { border-radius: 0 8px 8px 0; margin-left: -1px; }
  .seg button.on { background: var(--accent); color: var(--accent-ink); border-color: var(--accent); }
  .link { border: none; background: none; padding: 0; color: var(--muted); text-decoration: underline; font-size: inherit; }
</style>
