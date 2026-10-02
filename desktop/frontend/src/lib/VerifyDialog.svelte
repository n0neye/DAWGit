<script lang="ts">
  import { t } from "./i18n.svelte";
  import { Events } from "@wailsio/runtime";
  import Modal from "./Modal.svelte";
  import ProgressBar from "./ProgressBar.svelte";
  import { api, errorText, type Progress } from "./api";
  import type { VerifyResult } from "../../bindings/dawgit/desktop/models";

  // Check a project's history: every version, folder list and stored file.
  // Problems found can then be repaired (from the project folder or the
  // team's storage).
  let { root, name, onclose }: { root: string; name: string; onclose: () => void } = $props();

  let result = $state<VerifyResult | null>(null);
  let repaired = $state(false);
  let busy = $state(false);
  let error = $state("");
  let progress = $state<Progress | null>(null);

  $effect(() => Events.On("progress", (ev: { data: Progress }) => {
    if (busy && ev.data.root === root) progress = ev.data.stage === "done" ? null : ev.data;
  }));

  async function check(repair: boolean) {
    busy = true;
    error = "";
    progress = null;
    try {
      result = await api.VerifyProject(root, repair);
      repaired = repair;
    } catch (e) {
      error = errorText(e);
    } finally {
      busy = false;
      progress = null;
    }
  }
  $effect(() => { check(false); });

  let open = $derived(result?.problems.filter((p) => !p.fixed).length ?? 0);
  const kindName = (k: string) => ({ version: t("Version"), "folder-list": t("Folder list"), file: t("File") } as Record<string, string>)[k];
</script>

<Modal title={t("Check “{name}”", { name })} {onclose} backdropCloses={!busy} width={620}>
  {#if busy}
    <p class="muted">{repaired || result ? t("Repairing…") : t("Reading every version and stored file again…")}</p>
    <ProgressBar p={progress} waiting={t("Reading versions…")} />
  {:else if error}
    <p class="error">{error}</p>
  {:else if result}
    <p class:ok={result.problems.length === 0}>{result.summary}</p>
    {#if result.inTeam && !result.teamChecked}
      <p class="muted small">{t("The team's storage couldn't be reached: files only it keeps weren't checked.")}</p>
    {/if}
    {#if result.problems.length}
      <ul class="problems">
        {#each result.problems as p}
          <li class:fixed={p.fixed}>
            <span class="mark">{p.fixed ? "✓" : "!"}</span>
            <div>
              <div><span class="kind">{kindName(p.kind) ?? p.kind}</span> <span class="mono">{p.what}</span></div>
              <div class="muted small">{p.detail}{p.how ? ` — ${p.fixed ? t("repaired:") + " " : ""}${p.how}` : ""}</div>
            </div>
          </li>
        {/each}
      </ul>
    {/if}
    {#if open && !repaired}
      <p class="muted small">{t("Repair brings back what it can: from files in the project folder with the same content, or from the team's storage.")}</p>
    {:else if open}
      <p class="muted small">{t("What couldn't be repaired is lost from those versions; other versions are fine. Your project folder isn't changed.")}</p>
    {/if}
  {/if}
  {#snippet footer()}
    {#if result && open && !repaired && !busy}
      <button class="primary" onclick={() => check(true)}>{t("Repair")}</button>
    {/if}
    <button onclick={onclose} disabled={busy}>{result && !busy ? t("Done") : t("Cancel")}</button>
  {/snippet}
</Modal>

<style>
  .ok { color: var(--add); }
  .error { color: var(--danger); }
  .small { font-size: 12px; }
  .problems { list-style: none; margin: 10px 0; padding: 0; max-height: 50vh; overflow: auto; display: flex;
    flex-direction: column; gap: 8px; }
  .problems li { display: flex; gap: 8px; align-items: flex-start; }
  .mark { width: 16px; flex: none; text-align: center; font-weight: 700; color: var(--warn); }
  .fixed .mark { color: var(--add); }
  .kind { font-weight: 600; }
  .mono { word-break: break-all; }
</style>
