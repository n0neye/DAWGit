<script lang="ts">
  import { t, tn } from "./i18n.svelte";
  import type { State } from "./api";
  import Modal from "./Modal.svelte";
  import ProjectCheck from "./ProjectCheck.svelte";

  // A project with versions just joined a team: share them now, or later
  // (from the banner) after a look through the files.
  let { st, onshare, onclose }: { st: State; onshare: () => void; onclose: () => void } = $props();
  let isLive = $derived(st.tool === "Ableton Live");
</script>

<Modal title={t("Share “{name}” with {team}?", { name: st.name, team: st.teamName || t("the team") })} {onclose}>
  <p>{isLive ? tn(st.history.length, "Upload its {n} version now, samples included?", "Upload its {n} versions now, samples included?")
    : tn(st.history.length, "Upload its {n} version now?", "Upload its {n} versions now?")}</p>
  <ProjectCheck root={st.root} mode="added" />
  <p class="muted">{t("Or later: first look through the files and ignore the folders or files you don't need (right-click › Ignore), then share from the banner at the top. Your team sees the project once it's shared.")}{st.changes.length
      ? " " + tn(st.changes.length, "The {n} uncommitted change stays in Changes either way.", "The {n} uncommitted changes stay in Changes either way.") : ""}</p>
  {#snippet footer()}
    <button onclick={onclose}>{t("Later")}</button>
    <button class="primary" onclick={onshare}>{t("Share now")}</button>
  {/snippet}
</Modal>
