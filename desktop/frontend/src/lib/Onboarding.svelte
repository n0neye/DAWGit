<script lang="ts">
  import { t } from "./i18n.svelte";
  import { untrack } from "svelte";
  import { Events } from "@wailsio/runtime";
  import ProgressBar from "./ProgressBar.svelte";
  import { api, errorText, savedAuthor, rememberAuthor, type Overview, type Progress,
    type TeamSummary } from "./api";
  import JoinOrCreate from "./JoinOrCreate.svelte";
  import IdentityForm from "./IdentityForm.svelte";

  // First run: 1) join or create your team, 2) your name, 3) get or add projects.
  let { overview, reload, onfinish }: {
    overview: Overview;
    reload: () => Promise<void>;
    onfinish: (root?: string, share?: boolean) => void;
  } = $props();

  let step = $state(1);
  let team = $state<TeamSummary | null>(null);
  let name = $state(untrack(() => overview.author) || savedAuthor());
  let parent = $state("");
  let busy = $state("");
  let error = $state("");
  let downloaded = $state<string[]>([]);
  let progress = $state<Progress | null>(null);

  // Progress of the download in flight (only one runs at a time).
  $effect(() => Events.On("progress", (ev: { data: Progress }) => {
    if (busy) progress = ev.data.stage === "done" ? null : ev.data;
  }));

  async function connected(t: TeamSummary) {
    team = t;
    await reload();
    step = 2;
  }

  async function saveName() {
    busy = "name";
    try {
      await api.SetAuthor(name.trim());
      rememberAuthor(name.trim());
      await reload();
      step = team ? 3 : 4;
    } catch (e) {
      error = errorText(e);
    } finally {
      busy = "";
    }
  }

  async function pickParent() {
    parent = (await api.ChooseFolder(t("Where should downloaded projects go?"))) || parent;
  }

  async function download(id: string) {
    if (!parent) await pickParent();
    if (!parent || !team) return;
    busy = id;
    error = "";
    progress = null;
    try {
      const p = await api.DownloadProject(team.id, id, parent);
      downloaded = [...downloaded, p.root];
      await reload();
    } catch (e) {
      error = errorText(e);
    } finally {
      busy = "";
      progress = null;
    }
  }

  async function addFolder() {
    const folder = await api.ChooseFolder(t("Choose a project folder to share with the team"));
    if (!folder || !team) return;
    busy = "add";
    error = "";
    try {
      // Open it right away: its view uploads the first version and shows how
      // that is going.
      const p = await api.AddProjectToTeam(team.id, folder);
      onfinish(p.root, true);
    } catch (e) {
      error = errorText(e);
    } finally {
      busy = "";
    }
  }

  let remote = $derived(overview.projects.filter((p) => p.status === "remote"));
  let mine = $derived(overview.projects.filter((p) => p.status === "downloaded"));
</script>

<div class="onboarding">
  <div class="card" class:wide={step === 1}>
    <div class="brand"><img src="/icon.png" alt="" /> DAWGit</div>
    <ol class="steps">
      <li class:on={step === 1} class:done={step > 1}>{t("Team")}</li>
      <li class:on={step === 2} class:done={step > 2}>{t("Your name")}</li>
      <li class:on={step >= 3}>{t("Projects")}</li>
    </ol>

    {#if step === 1}
      <h1>{t("Your team")}</h1>
      <p class="muted">{t("DAWGit keeps your projects' versions in your team's storage, safe if a drive fails. One person creates the team; everyone else joins with the connection code they send. Working alone? Create a team of one.")}</p>
      <JoinOrCreate onconnected={connected} />
    {:else if step === 2 && team}
      <h1>{t("Who are you in {team}?", { team: team.name })}</h1>
      <IdentityForm {team} suggested={name} onsaved={async (t) => {
        team = t;
        rememberAuthor(t.memberName);
        await reload();
        step = 3;
      }} />
    {:else if step === 2}
      <h1>{t("What should your teammates call you?")}</h1>
      <p class="muted">{t("Your name appears next to the versions you save.")}</p>
      <form onsubmit={(e) => { e.preventDefault(); saveName(); }}>
        <input bind:value={name} placeholder={t("e.g. Yi")} />
        <div class="row actions">
          <span class="spacer"></span>
          <button type="submit" class="primary" disabled={!name.trim() || !!busy}>{t("Continue")}</button>
        </div>
      </form>
    {:else if step === 3 && team}
      <h1>{t("Projects in {team}", { team: team.name })}</h1>
      {#if overview.teamError}<p class="error">{overview.teamError}</p>{/if}
      {#if remote.length === 0 && mine.length === 0}
        <p class="muted">{t("The team has no projects yet. Add yours to start.")}</p>
      {:else}
        <p class="muted">{t("Download the songs you work on. You can get the others later from the sidebar.")}</p>
        <div class="row parent">
          <span class="faint">{t("Download into")}</span>
          <span class="mono path">{parent || `— ${t("choose a folder")} —`}</span>
          <button class="ghost" onclick={pickParent}>{t("Browse…")}</button>
        </div>
        <ul class="projects">
          {#each mine as p (p.id)}
            <li><span class="name">{p.name}</span><span class="ok">{t("✓ on this computer")}</span></li>
          {/each}
          {#each remote as p (p.id)}
            <li class:active={busy === p.id}>
              <span class="name">{p.name}</span>
              <button onclick={() => download(p.id)} disabled={!!busy}>{busy === p.id ? t("Downloading…") : `↓ ${t("Download")}`}</button>
              {#if busy === p.id}
                <div class="progress"><ProgressBar p={progress} waiting={t("Connecting…")} /></div>
              {/if}
            </li>
          {/each}
        </ul>
      {/if}
      <p class="faint tip">{t("Tip: to stay out of each other's way, each of you can work on your own branch (⑂ → New branch) or your own set, and merge when it's ready.")}</p>
      {#if error}<p class="error">{error}</p>{/if}
      <div class="row actions">
        <button onclick={addFolder} disabled={!!busy}>{busy === "add" ? t("Sharing…") : `+ ${t("Add a project")}`}</button>
        <span class="spacer"></span>
        <button class="primary" onclick={() => onfinish(downloaded[0])}>
          {mine.length || downloaded.length ? t("Done") : t("Skip for now")}
        </button>
      </div>
    {:else}
      <h1>{t("You're set")}</h1>
      <p class="muted">{t("Your project is tracked on this computer. Connect to a team any time from the sidebar to share it.")}</p>
      <div class="row actions">
        <span class="spacer"></span>
        <button class="primary" onclick={() => onfinish(downloaded[0])}>{t("Open DAWGit")}</button>
      </div>
    {/if}
  </div>
</div>

<style>
  .onboarding { height: 100%; display: flex; align-items: center; justify-content: center; padding: 24px; overflow: auto; }
  .card { width: 560px; max-width: 100%; background: var(--panel); border: 1px solid var(--line); border-radius: 14px; padding: 26px 30px; }
  .card.wide { width: 660px; }
  .brand { display: flex; align-items: center; gap: 8px; font-weight: 700; margin-bottom: 14px; }
  .brand img { width: 24px; height: 24px; }
  .steps { list-style: none; display: flex; gap: 8px; padding: 0; margin: 0 0 18px; font-size: 12px; }
  .steps li { padding: 3px 10px; border-radius: 10px; background: var(--bg); color: var(--faint); }
  .steps li.on { background: #1f3b35; color: var(--accent); }
  .steps li.done { color: var(--muted); }
  .steps li.done::before { content: "✓ "; }
  h1 { font-size: 20px; margin: 0 0 6px; }
  .actions { margin-top: 18px; }
  .error { color: var(--danger); }
  .parent { margin: 12px 0 8px; }
  .path { flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .projects { list-style: none; padding: 0; margin: 0; display: flex; flex-direction: column; gap: 6px; max-height: 240px; overflow: auto; }
  .projects li { display: flex; flex-wrap: wrap; align-items: center; gap: 10px; padding: 8px 12px; border: 1px solid var(--line); border-radius: 8px; background: var(--bg); }
  .name { flex: 1; font-weight: 600; }
  .ok { color: var(--accent); font-size: 13px; }
  .projects li.active { border-color: #2c4557; }
  .progress { flex-basis: 100%; display: flex; }
  .tip { font-size: 12.5px; margin: 14px 0 0; }
</style>
