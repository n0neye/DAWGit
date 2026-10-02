<script lang="ts">
  import Modal from "./Modal.svelte";
  import ChangeList from "./ChangeList.svelte";
  import { ago, type Preview } from "./api";

  // Teammates committed on this branch while you were working: combine your
  // work with theirs (after seeing what comes in), or put yours on a branch.
  // older: the changes were made on an older version (Go to), not while
  // teammates committed.
  let { preview, branch, older = false, message = $bindable(), busy = false, oncombine, onbranch, onclose }: {
    preview: Preview;
    branch: string;
    older?: boolean;
    message: string;
    busy?: boolean;
    oncombine: () => void;
    onbranch: () => void;
    onclose: () => void;
  } = $props();

  // Merges only combine the other versions: not listed (unless that's all).
  let versions = $derived.by(() => {
    const own = preview.versions.filter((v) => v.parents.length < 2);
    return own.length ? own : preview.versions;
  });
  let authors = $derived([...new Set(versions.map((v) => v.author))].join(", ") || "Your team");
  let n = $derived(versions.length);
</script>

<Modal title={older ? `“${branch}” has ${n} newer version${n === 1 ? "" : "s"} than the one you're on`
  : `${authors} committed ${n} version${n === 1 ? "" : "s"} while you were working`} {onclose} width={720}>
  {#if older}
    <p class="muted">You changed an older version. Commit your changes after it and combine them with the latest
      version of “{branch}” — DAWGit merges track by track and asks only where both changed the same thing — or
      keep your work on a branch of its own.</p>
  {:else}
    <p class="muted">Your changes are not committed yet. Combine them with the team's versions on “{branch}” —
      DAWGit merges track by track and asks only where you both changed the same thing — or keep your work on a
      branch of its own for now.</p>
  {/if}

  <h3>New on “{branch}”</h3>
  <ul class="versions">
    {#each versions as v (v.id)}
      <li><span class="msg">{v.message || "(no description)"}</span>
        <span class="faint">{v.author} · {ago(v.time)}</span></li>
    {/each}
  </ul>
  <h3>What they changed</h3>
  <ChangeList changes={preview.changes} />
  {#if preview.conflicts.length}
    <div class="conflicts">
      <strong>{preview.conflicts.length} thing{preview.conflicts.length === 1 ? "" : "s"} you also changed in
        committed versions</strong> — you'll choose what to keep next:
      <ul>
        {#each preview.conflicts as c (c.key)}
          <li>{c.unit === c.file ? c.file : `${c.unit} (${c.file})`}</li>
        {/each}
      </ul>
    </div>
  {:else}
    <p class="note">If one of your uncommitted changes touches a track they changed too, you'll choose what to keep
      next.</p>
  {/if}

  <label for="cm">Describe your changes</label>
  <input id="cm" bind:value={message} placeholder="What did you change? e.g. “New bassline in the chorus”" />

  {#snippet footer()}
    <button onclick={onclose}>Cancel</button>
    <button onclick={onbranch} disabled={busy || !message.trim()}
      title="Commit your work on a new branch; “{branch}” stays as it is">Put my work on a new branch…</button>
    <button class="primary" onclick={oncombine} disabled={busy || !message.trim()}>Combine and share</button>
  {/snippet}
</Modal>

<style>
  h3 { font-size: 13px; text-transform: uppercase; letter-spacing: .06em; color: var(--muted); margin: 14px 0 8px; }
  .versions { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 4px; }
  .versions li { display: flex; gap: 10px; }
  .msg { flex: 1; }
  .conflicts {
    margin-top: 14px; padding: 10px 12px; border-radius: 8px;
    background: var(--warn-bg); border: 1px solid #5a4623; color: #f0d9a8;
  }
  .conflicts ul { margin: 6px 0 0; padding-left: 20px; }
  .note { margin-top: 14px; color: var(--muted); font-size: 13px; }
  label { display: block; margin-top: 16px; }
  input { width: 100%; }
</style>
