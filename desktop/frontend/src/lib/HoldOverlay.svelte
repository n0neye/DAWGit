<script lang="ts">
  import { t } from "./i18n.svelte";
  import type { Progress } from "./api";
  import ProgressBar from "./ProgressBar.svelte";

  // While DAWGit reads the project's files for a version (looking for
  // changes, adding them to the history), the page is covered: saving in
  // the tool now could put old and new files in the same version. Once
  // they're in the history (the upload is next), it goes away and the
  // banner says to go on working.
  let { p, tool, team }: { p: Progress; tool: string; team?: string } = $props();
</script>

<div class="hold" role="alertdialog" aria-modal="true" aria-labelledby="hold-title">
  <div class="card">
    <h2 id="hold-title">✋ {t("Don't change the project yet")}</h2>
    <p>{t("DAWGit is reading the project's files for the version. Wait to save in {tool} until this is done: it only takes a moment, and the upload after it doesn't need you to wait.", { tool })}</p>
    <ProgressBar {p} {team} />
  </div>
</div>

<style>
  .hold {
    position: absolute; inset: 0; z-index: 30; display: flex; align-items: center; justify-content: center;
    background: rgba(15, 16, 19, .72); backdrop-filter: blur(2px);
  }
  .card {
    width: min(460px, calc(100% - 48px)); padding: 20px 22px; border-radius: 12px;
    background: var(--panel-2); border: 1px solid #5a4623; box-shadow: 0 16px 40px rgba(0, 0, 0, .5);
    display: flex; flex-direction: column; gap: 12px;
  }
  h2 { margin: 0; font-size: 17px; color: #f0d9a8; }
  p { margin: 0; font-size: 13.5px; color: var(--muted); }
</style>
