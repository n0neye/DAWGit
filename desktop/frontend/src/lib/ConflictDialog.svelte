<script lang="ts">
  import Modal from "./Modal.svelte";
  import type { Conflict } from "./api";

  let { conflicts, onresolve, onclose }: {
    conflicts: Conflict[];
    onresolve: (resolutions: Record<string, string>) => void;
    onclose: () => void;
  } = $props();

  let choices = $state<Record<string, string>>({});
  let complete = $derived(conflicts.every((c) => choices[c.key]));

  function all(choice: string) {
    for (const c of conflicts) {
      choices[c.key] = choice === "both" && !c.canKeepBoth ? "ours" : choice;
    }
  }
</script>

<Modal title="You and the team changed the same things" {onclose} width={680}>
  <p class="muted">
    Everything else was combined automatically. Choose what to keep for each of these:
  </p>
  <div class="bulk row">
    <span class="faint">All:</span>
    <button class="ghost" onclick={() => all("ours")}>Keep mine</button>
    <button class="ghost" onclick={() => all("theirs")}>Take theirs</button>
    <button class="ghost" onclick={() => all("both")}>Keep both</button>
  </div>
  <ul>
    {#each conflicts as c (c.key)}
      <li>
        <div class="what">
          <div class="unit">{c.unit === c.file ? c.file : c.unit}</div>
          <div class="faint">{c.unit === c.file ? "" : c.file + " · "}{c.description}</div>
        </div>
        <div class="seg" role="radiogroup" aria-label={c.unit}>
          <button class:on={choices[c.key] === "ours"} onclick={() => (choices[c.key] = "ours")}>Keep mine</button>
          <button class:on={choices[c.key] === "theirs"} onclick={() => (choices[c.key] = "theirs")}>Take theirs</button>
          {#if c.canKeepBoth}
            <button class:on={choices[c.key] === "both"} onclick={() => (choices[c.key] = "both")}
              title="Keeps your version and adds theirs as a copy">Keep both</button>
          {/if}
        </div>
      </li>
    {/each}
  </ul>
  {#snippet footer()}
    <button onclick={onclose}>Cancel</button>
    <button class="primary" disabled={!complete} onclick={() => onresolve({ ...choices })}>Continue</button>
  {/snippet}
</Modal>

<style>
  ul { list-style: none; padding: 0; margin: 8px 0 0; display: flex; flex-direction: column; gap: 8px; }
  li {
    display: flex; align-items: center; gap: 14px; padding: 10px 12px;
    background: var(--bg); border: 1px solid var(--line); border-radius: 8px;
  }
  .what { flex: 1; min-width: 0; }
  .unit { font-weight: 600; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .seg { display: flex; }
  .seg button { border-radius: 0; margin-left: -1px; padding: 5px 10px; font-size: 13px; }
  .seg button:first-child { border-radius: 6px 0 0 6px; }
  .seg button:last-child { border-radius: 0 6px 6px 0; }
  .seg button.on { background: var(--accent); color: var(--accent-ink); border-color: var(--accent); }
  .bulk { margin: 4px 0; }
  .bulk button { padding: 2px 8px; font-size: 13px; }
</style>
