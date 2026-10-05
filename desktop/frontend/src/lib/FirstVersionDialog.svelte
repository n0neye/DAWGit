<script lang="ts">
  import { t, tn } from "./i18n.svelte";
  import type { State } from "./api";
  import Modal from "./Modal.svelte";
  import ProjectCheck from "./ProjectCheck.svelte";

  // A project just added, with no versions yet: commit (and share) a first
  // one now, or after a look through the files.
  let { st, message = $bindable(), oncommit, onclose }: {
    st: State; message: string; oncommit: () => void; onclose: () => void;
  } = $props();
  let isLive = $derived(st.tool === "Ableton Live");
</script>

<Modal title={t("“{name}” is added", { name: st.name })} {onclose}>
  <p>{st.remoteUrl
    ? t(isLive ? "Commit a first version now and share it with {team}, samples included?" : "Commit a first version now and share it with {team}?", { team: st.teamName || t("the team") })
    : t("Commit a first version now?")}</p>
  <ProjectCheck root={st.root} mode="added" />
  <p class="muted later" title={tn(st.changes.length, "Or later: first look through the {n} file and ignore the folders or files you don't need (right-click › Ignore), then commit from the Changes tab.",
    "Or later: first look through the {n} files and ignore the folders or files you don't need (right-click › Ignore), then commit from the Changes tab.")}>{t("Or later, from the Changes tab: you can leave files out first.")}</p>
  <label for="first-msg">{t("Message")}</label>
  <input id="first-msg" bind:value={message} onkeydown={(e) => { if (e.key === "Enter" && message.trim()) oncommit(); }} />
  {#snippet footer()}
    <button onclick={onclose}>{t("Later")}</button>
    <button class="primary" disabled={!message.trim()} onclick={oncommit}>
      {st.remoteUrl ? t("Commit & Share now") : t("Commit now")}</button>
  {/snippet}
</Modal>
