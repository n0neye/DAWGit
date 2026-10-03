<script lang="ts">
  import { t, tn } from "./i18n.svelte";
  import Modal from "./Modal.svelte";
  import IncomingChanges from "./IncomingChanges.svelte";
  import { type Preview } from "./api";

  // Teammates committed on this branch while you were working: combine your
  // work with theirs (after seeing what comes in), or put yours on a branch.
  // older: the changes were made on an older version (Go to), not while
  // teammates committed.
  let { root, preview, branch, older = false, message = $bindable(), busy = false, oncombine, onbranch, onclose }: {
    root: string;
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
  let authors = $derived([...new Set(versions.map((v) => v.author))].join(", ") || t("Your team"));
  let n = $derived(versions.length);
</script>

<Modal width={1000} title={older ? tn(n, "“{branch}” has {n} newer version than the one you're on", "“{branch}” has {n} newer versions than the one you're on", { branch })
  : tn(n, "{who} committed {n} version while you were working", "{who} committed {n} versions while you were working", { who: authors })} {onclose}>
  {#if older}
    <p class="muted">{t("You changed an older version. Commit your changes after it and combine them with the latest version of “{branch}” — DAWGit merges track by track and asks only where both changed the same thing — or keep your work on a branch of its own.", { branch })}</p>
  {:else}
    <p class="muted">{t("Your changes are not committed yet. Combine them with the team's versions on “{branch}” — DAWGit merges track by track and asks only where you both changed the same thing — or keep your work on a branch of its own for now.", { branch })}</p>
  {/if}

  <h3>{t("New on “{branch}”", { branch })}</h3>
  <IncomingChanges {root} {preview} {versions} />
  {#if preview.conflicts.length}
    <div class="conflicts">
      <strong>{tn(preview.conflicts.length, "{n} thing you also changed in committed versions", "{n} things you also changed in committed versions")}</strong> {t("— you'll choose what to keep next:")}
      <ul>
        {#each preview.conflicts as c (c.key)}
          <li>{c.unit === c.file ? c.file : `${c.unit} (${c.file})`}</li>
        {/each}
      </ul>
    </div>
  {:else}
    <p class="note">{t("If one of your uncommitted changes touches a track they changed too, you'll choose what to keep next.")}</p>
  {/if}

  <label for="cm">{t("Describe your changes")}</label>
  <input id="cm" bind:value={message} placeholder={t("What did you change? e.g. “New bassline in the chorus”")} />

  {#snippet footer()}
    <button onclick={onclose}>{t("Cancel")}</button>
    <button onclick={onbranch} disabled={busy || !message.trim()}
      title={t("Commit your work on a new branch; “{branch}” stays as it is", { branch })}>{t("Put my work on a new branch…")}</button>
    <button class="primary" onclick={oncombine} disabled={busy || !message.trim()}>{t("Combine and share")}</button>
  {/snippet}
</Modal>

<style>
  h3 { font-size: 13px; text-transform: uppercase; letter-spacing: .06em; color: var(--muted); margin: 14px 0 8px; }
  .conflicts {
    margin-top: 14px; padding: 10px 12px; border-radius: 8px;
    background: var(--warn-bg); border: 1px solid #5a4623; color: #f0d9a8;
  }
  .conflicts ul { margin: 6px 0 0; padding-left: 20px; }
  .note { margin-top: 14px; color: var(--muted); font-size: 13px; }
  label { display: block; margin-top: 16px; }
  input { width: 100%; }
</style>
