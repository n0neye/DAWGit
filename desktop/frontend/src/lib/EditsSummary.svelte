<script lang="ts">
  import { t } from "./i18n.svelte";
  import type { State } from "./api";

  // Beside the Changes list when no file is picked: the tracks you changed
  // (Live sets), or what to do.
  let { st }: { st: State } = $props();
  let tool = $derived(st.tool === "Ableton Live" ? "Live" : st.tool ? t(st.tool) : t("your app"));
</script>

<section>
  {#if st.myEdits.length}
    <h3>{t("Tracks you changed")}</h3>
    <ul class="tracks">
      {#each st.myEdits as e}
        <li>
          <span class="chg {e.change}"></span>
          <span>{e.name}</span>
          <span class="faint">{e.set}</span>
        </li>
      {/each}
    </ul>
    <p class="faint small">{t("Pick a file on the left for its details, history and, for samples, to listen.")}</p>
  {:else}
    <p class="muted">{st.changes.length ? t("Pick a file on the left to see what changed.") : t("No uncommitted changes. Work in {tool} and save (Ctrl+S) — your changes show up here.", { tool })}</p>
  {/if}
</section>

<style>
  h3 { font-size: 12px; text-transform: uppercase; letter-spacing: .06em; color: var(--muted); margin: 6px 0 10px; }
  .small { font-size: 12px; margin: 10px 0 0; }
  .tracks { list-style: none; padding: 0; margin: 0 0 18px; display: flex; flex-direction: column; gap: 4px; }
  .tracks li { display: flex; align-items: center; gap: 10px; }
  .chg { width: 8px; height: 8px; border-radius: 2px; background: var(--mod); }
  .chg.added { background: var(--add); }
  .chg.removed { background: var(--del); }
</style>
