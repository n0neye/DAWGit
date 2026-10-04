<script lang="ts">
  import { t } from "./i18n.svelte";
  import { untrack } from "svelte";
  import { api, errorText, type Member, type TeamSummary } from "./api";

  // Who you are in a team. On a new computer, pick yourself from the member
  // list (same person, same name on all your versions) or join as someone
  // new. Once set, this renames you for everyone, on old versions too.
  let { team, suggested = "", submitLabel = "Continue", askShare = false, onsaved }: {
    team: TeamSummary;
    suggested?: string; // a name to start with (e.g. from this computer)
    submitLabel?: string;
    askShare?: boolean; // also ask whether to share this computer's setup (joining)
    onsaved: (t: TeamSummary) => void;
  } = $props();

  let members = $state<Member[] | null>(null);
  let pick = $state(untrack(() => team.memberId) || "new"); // member id, or "new"
  let name = $state(untrack(() => team.memberName) || untrack(() => suggested));
  let busy = $state(false);
  let share = $state(true);
  let sharing = $derived(askShare && team.canShareSetup && !team.memberId);
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
      // First the choice (shared once you are someone in the team).
      if (sharing) await api.SetShareSetup(team.id, share);
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
    <p class="muted">{t("Already in {team} on another computer? Pick yourself, so all your versions show one name.", { team: team.name })}</p>
    <ul class="members">
      {#each others as m (m.id)}
        <li><label><input type="radio" name="who" checked={pick === m.id} onchange={() => choose(m.id)} /> {m.name}</label></li>
      {/each}
      <li><label><input type="radio" name="who" checked={pick === "new"} onchange={() => { pick = "new"; name = suggested; }} />
        {t("I'm new to this team")}</label></li>
    </ul>
  {/if}
  <label for="who-name">{renaming ? t("Your name in {team}", { team: team.name }) : t("Your name")}</label>
  <input id="who-name" bind:value={name} placeholder={t("e.g. Yi")} autocomplete="off" />
  <p class="faint small">{t("Shown next to the versions you commit. If you change it later, it changes on all your versions, for everyone in the team.")}</p>
  {#if sharing}
    <label class="share"><input type="checkbox" bind:checked={share} /> {t("Share my setup with the team")}</label>
    <p class="faint small">{t("Your Ableton Live version and the names of your plugins and packs (never files), so a project check can tell who can open a project.")} {t("You can change this in the team's settings.")}</p>
  {/if}
  {#if error}<p class="error">{error}</p>{/if}
  <div class="row actions">
    <span class="spacer"></span>
    <button type="submit" class="primary" disabled={!name.trim() || busy || members === null}>
      {busy ? t("Saving…") : submitLabel}
    </button>
  </div>
</form>

<style>
  .members { list-style: none; padding: 0; margin: 8px 0 14px; display: flex; flex-direction: column; gap: 6px; }
  .members label { display: flex; align-items: center; gap: 8px; margin: 0; color: var(--text); font-size: 14px; }
  .members input { width: auto; }
  .small { font-size: 12px; margin: 6px 0 0; }
  .error { color: var(--danger); }
  .share { display: flex; align-items: center; gap: 8px; margin: 14px 0 0; color: var(--text); font-size: 14px; }
  .share input { width: auto; }
  .actions { margin-top: 14px; }
</style>
