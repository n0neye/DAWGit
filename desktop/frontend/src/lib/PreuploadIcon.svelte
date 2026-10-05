<script lang="ts">
  import { t } from "./i18n.svelte";
  import type { Preupload } from "./preupload.svelte";

  // A small moving cloud while a big file of the project goes up in the
  // background; what and how far in its tooltip.
  let { p }: { p: Preupload } = $props();
  let file = $derived(p.path.slice(p.path.lastIndexOf("/") + 1));
  let pct = $derived(p.total ? Math.round((100 * p.bytes) / p.total) : 0);
</script>

<span class="pre" role="img" aria-label={t("Uploading {file} in the background ({percent}%)", { file, percent: pct })}
  title={t("Uploading {file} in the background ({percent}%)", { file, percent: pct })}>
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
    <path d="M7 18a4.5 4.5 0 0 1-.6-8.96A6 6 0 0 1 18 8.5a4 4 0 0 1-.5 9.5"/>
    <g class="arrow"><path d="M12 20v-7"/><path d="m9 15 3-3 3 3"/></g>
  </svg>
</span>

<style>
  .pre { display: inline-flex; margin-left: 4px; color: var(--accent); vertical-align: middle; }
  svg { width: 14px; height: 14px; overflow: visible; }
  .arrow { animation: rise 1.4s ease-in-out infinite; }
  @keyframes rise {
    0% { transform: translateY(3px); opacity: 0; }
    40% { opacity: 1; }
    100% { transform: translateY(-3px); opacity: 0; }
  }
</style>
