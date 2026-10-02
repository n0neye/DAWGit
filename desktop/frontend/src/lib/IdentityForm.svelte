<script lang="ts">
  import { t } from "./i18n.svelte";
  import { untrack } from "svelte";
  import { api, errorText, type Member, type TeamSummary } from "./api";

  // Who you are in a team. On a new computer, pick yourself from the member
  // list (same person, same name on all your versions) or join as someone
  // new. Once set, this renames you for everyone, on old versions too.
  let { team, suggested = "", submitLabel = "Continue", onsaved }: {
    team: TeamSummary;
    suggested?: string; // a name to start with (e.g. from this computer)
    submitLabel?: string;
    onsaved: (t: TeamSummary) => void;
  } = $props();

  let members = $state<Member[] | null>(null);
  let pick = $state(untrack(() => team.memberId) || "new"); // member id, or "new"
  let name = $state(untrack(() => team.memberName) || untrack(() => suggested));
  let busy = $state(false);
  let error = $state("");

  $effect(() => {
    const id = team.id;
    api.TeamMembers(id).then((ms) => (members = ms ?? [])).catch(() => (members = []));
  });

  let renaming = $derived(!!team.memberId);
  let others = $derived((members ?? []).filter((m) => m.id !== team.memberId));

  function choose(id: string) {
    pick = id;
    const m = members?.find((x) => x.id === id);
    if (m) name = m.name;
  }

  async function save() {
    busy = true;
    error = "";
    try {
      onsaved(await api.SetIdentity(team.id, pick === "new" ? "" : pick, name.trim()));
    } catch (e) {
      error = errorText(e);
    } finally {
      busy = false;
    }
  }
</script>

<form onsubmit={(e) => { e.preventDefault(); save(); }}>
  {#if !renaming && others.length}
    <p class="muted">Already in {team.name} on another computer? Pick yourself, so all your versions show one name.</p>
    <ul class="members">
      {#each others as m (m.id)}
        <li><label><input type="radio" name="who" checked={pick === m.id} onchange={() => choose(m.id)} /> {m.name}</label></li>
      {/each}
      <li><label><input type="radio" name="who" checked={pick === "new"} onchange={() => { pick = "new"; name = suggested; }} />
        {t("I'm new to this team")}</label></li>
    </ul>
  {/if}
  <label for="who-name">{renaming ? `Your name in ${team.name}` : "Your name"}</label>
  <input id="who-name" bind:value={name} placeholder={t("e.g. Yi")} autocomplete="off" />
  <p class="faint small">{t("Shown next to the versions you commit. If you change it later, it changes on all your versions, for everyone in the team.")}</p>
  {#if error}<p class="error">{error}</p>{/if}
  <div class="row actions">
    <span class="spacer"></span>
    <button type="submit" class="primary" disabled={!name.trim() || busy || members === null}>
      {busy ? "Saving…" : submitLabel}
    </button>
  </div>
</form>

<style>
  .members { list-style: none; padding: 0; margin: 8px 0 14px; display: flex; flex-direction: column; gap: 6px; }
  .members label { display: flex; align-items: center; gap: 8px; margin: 0; color: var(--text); font-size: 14px; }
  .members input { width: auto; }
  .small { font-size: 12px; margin: 6px 0 0; }
  .error { color: var(--danger); }
  .actions { margin-top: 14px; }
</style>
