<script lang="ts">
  import { lineKind, type Change } from "./api";

  let { changes, empty = "No changes" }: { changes: Change[]; empty?: string } = $props();

  const sym: Record<string, string> = { added: "+", modified: "~", deleted: "−", untracked: "○" };
</script>

{#if changes.length === 0}
  <p class="muted">{empty}</p>
{:else}
  <ul class="changes">
    {#each changes as c (c.path)}
      <li>
        <div class="file {c.status}"><span class="sym">{sym[c.status] ?? "·"}</span>{c.path}</div>
        {#if c.details.length}
          <div class="details mono">
            {#each c.details as line}
              <div class={lineKind(line)} style:padding-left="{(line.length - line.trimStart().length) * 4 + 4}px">{line.trim()}</div>
            {/each}
          </div>
        {/if}
      </li>
    {/each}
  </ul>
{/if}

<style>
  .changes { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 10px; }
  .file { font-weight: 600; display: flex; gap: 8px; }
  .sym { width: 12px; text-align: center; }
  .file.added .sym { color: var(--add); }
  .file.deleted .sym { color: var(--del); }
  .file.modified .sym { color: var(--mod); }
  .details {
    margin: 6px 0 0 20px; padding: 8px 10px; background: var(--bg); border: 1px solid var(--line);
    border-radius: 6px; line-height: 1.6; user-select: text;
  }
  .add { color: var(--add); }
  .del { color: var(--del); }
  .mod { color: var(--mod); }
</style>
