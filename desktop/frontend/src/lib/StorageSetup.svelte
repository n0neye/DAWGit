<script lang="ts">
  import { t, tn } from "./i18n.svelte";
  import Tx from "./Tx.svelte";
  import { api, errorText, type TeamSummary } from "./api";
  import CodeBox from "./CodeBox.svelte";

  // Create a team on S3-compatible storage. Cloudflare R2 comes with a step
  // by step guide; any other S3-compatible storage takes the same fields.
  // DAWGit checks it all, then hands out the connection code for teammates,
  // and suggests backing the team up (unless a member already does).
  let { onconnected, oncreated }: {
    onconnected: (t: TeamSummary) => void;
    oncreated?: () => void; // the team exists; the code is on screen
  } = $props();

  const DASHBOARD = "https://dash.cloudflare.com/";

  let provider = $state<"r2" | "s3">("r2");
  let endpoint = $state("");
  let bucket = $state("");
  let accessKey = $state("");
  let secretKey = $state("");
  let name = $state("");
  let folder = $state("");
  let region = $state("");
  let busy = $state(false);
  let error = $state("");
  let created = $state<{ team: TeamSummary; code: string } | null>(null);

  // After the code: the backup step.
  let backupStep = $state(false);
  let backupFolder = $state("");
  let backupProblem = $state("");
  let backupBusy = $state(false);

  async function afterCode() {
    const team = created!.team;
    try {
      const info = await api.BackupInfo(team.id);
      if (info?.supported && !info.covered && !info.folder) {
        backupStep = true;
        return;
      }
    } catch {
      // can't tell: no backup step; Team Settings has it
    }
    onconnected(team);
  }

  async function chooseBackup() {
    backupProblem = error = "";
    backupBusy = true;
    try {
      const dir = await api.ChooseFolder(t("Choose a backup folder"));
      if (!dir) return;
      backupProblem = await api.SetBackupFolder(created!.team.id, dir);
      if (!backupProblem) backupFolder = dir;
    } catch (e) {
      error = errorText(e);
    } finally {
      backupBusy = false;
    }
  }

  let ready = $derived(!!(endpoint.trim() && bucket.trim() && accessKey.trim() && secretKey.trim() && name.trim()));

  async function create() {
    busy = true;
    error = "";
    try {
      const team = await api.CreateStorageTeam({ endpoint, bucket, folder, region, accessKey, secretKey }, name);
      created = { team, code: await api.TeamConnectionCode(team.id) };
      oncreated?.();
    } catch (e) {
      error = errorText(e);
    } finally {
      busy = false;
    }
  }
</script>

{#snippet keys(hint: string)}
  <label for="s-ak">{t("Access Key ID")}</label>
  <input id="s-ak" bind:value={accessKey} autocomplete="off" spellcheck="false" />
  <label for="s-sk">{t("Secret Access Key")}</label>
  <input id="s-sk" type="password" bind:value={secretKey} autocomplete="off" />
  <label for="s-ep">{t("Endpoint")} {#if hint}<span class="faint">{hint}</span>{/if}</label>
  <input id="s-ep" bind:value={endpoint} autocomplete="off" spellcheck="false"
    placeholder={provider === "r2" ? "https://<account id>.r2.cloudflarestorage.com" : "https://s3.<region>.amazonaws.com"} />
{/snippet}

{#snippet teamName()}
  <label for="s-name">{t("Team name")} <span class="faint">{t("(everyone sees it)")}</span></label>
  <input id="s-name" bind:value={name} placeholder={t("e.g. Night Shift")} />
{/snippet}

{#if created && backupStep}
  <div class="done">
    <h3 class="step-h">{t("Back up your team")}</h3>
    <p>{t("Everything is in your storage, but a second copy on your own drive or NAS keeps the team's work safe if the bucket or its keys are ever lost. Once a day while DAWGit is open, it copies what's new, and it never deletes anything from the backup.")}</p>
    {#if backupFolder}
      <p class="ok">✓ {t("Backing up to {folder}", { folder: backupFolder })}</p>
      <p class="faint small">{t("The first backup copies everything and can take a while; it carries on in the background. Change it any time in the team's settings (⚙) › Backup.")}</p>
    {:else}
      <p class="faint small">{t("Only one member needs to do this. You can also set it up later in the team's settings (⚙) › Backup.")}</p>
    {/if}
    {#if backupProblem === "other-team"}
      <p class="error small">{t("That folder holds another team's backup. Choose another one.")}</p>
    {:else if backupProblem === "not-empty"}
      <p class="error small">{t("That folder has other things in it. Choose an empty folder (or make a new one), or this team's earlier backup.")}</p>
    {/if}
    {#if error}<p class="error small">{error}</p>{/if}
    <div class="row actions">
      <span class="spacer"></span>
      {#if backupFolder}
        <button class="primary" onclick={() => onconnected(created!.team)}>{t("Continue")}</button>
      {:else}
        <button onclick={() => onconnected(created!.team)} disabled={backupBusy}>{t("Set up later")}</button>
        <button class="primary" onclick={chooseBackup} disabled={backupBusy}>{t("Choose a backup folder…")}</button>
      {/if}
    </div>
  </div>
{:else if created}
  <div class="done">
    <p class="ok">✓ {t("Storage checked — “{team}” is ready.", { team: created.team.name })}</p>
    {#if created.team.name !== name.trim()}
      <p class="faint small">{t("This bucket already holds the team “{team}”, so you joined it.", { team: created.team.name })}</p>
    {/if}
    <p><Tx text={t("Send this {code} to each teammate. They choose {join} and paste it.")} strong={{ code: t("connection code") }} em={{ join: t("Join a team") }} /></p>
    <CodeBox code={created.code} />
    <p class="faint small">{t("The code contains the storage key: send it privately (a direct message, not a public channel). You can copy it again later from the ⚙ next to the team in the Team menu.")}</p>
    <div class="row actions">
      <span class="spacer"></span>
      <button class="primary" onclick={afterCode}>{t("Continue")}</button>
    </div>
  </div>
{:else}
  <p class="muted intro">{t("Your team's songs live in a storage bucket of your own, so nobody has to keep a computer running. Any S3-compatible storage works; Cloudflare R2 is the easiest start (no download fees, and a small team usually stays within its free allowance).")}</p>

  <div class="provider" role="radiogroup" aria-label={t("Storage")}>
    <label class:on={provider === "r2"}><input type="radio" bind:group={provider} value="r2" /> Cloudflare R2
      <span class="faint">{t("guided, about 5 minutes")}</span></label>
    <label class:on={provider === "s3"}><input type="radio" bind:group={provider} value="s3" /> {t("Other S3-compatible")}
      <span class="faint">{t("Amazon S3, MinIO, …")}</span></label>
  </div>

  {#if provider === "r2"}
    <ol class="guide">
      <li>
        <div class="n">1</div>
        <div class="body">
          <h3>{t("Open R2 in Cloudflare")}</h3>
          <p><Tx text={t("Sign up or log in, then in the sidebar open {menu}. The first time, Cloudflare asks you to activate R2 (it may ask for a payment method even for the free allowance).")}
            strong={{ menu: "Storage & databases → R2 Object Storage" }} /></p>
          <button onclick={() => api.OpenURL(DASHBOARD)}>{t("Open the Cloudflare dashboard ↗")}</button>
        </div>
      </li>
      <li>
        <div class="n">2</div>
        <div class="body">
          <h3>{t("Create a bucket")}</h3>
          <p><Tx text={t("{create} → a name (e.g. {example}) → keep Location {auto} and Storage Class {std} → {create}. Use a bucket just for DAWGit.")}
            strong={{ create: "Create bucket" }} code={{ example: "night-shift-dawgit" }} em={{ auto: "Automatic", std: "Standard" }} /></p>
          <label for="s-bucket">{t("Bucket name")}</label>
          <input id="s-bucket" bind:value={bucket} placeholder="night-shift-dawgit" autocomplete="off" spellcheck="false" />
        </div>
      </li>
      <li>
        <div class="n">3</div>
        <div class="body">
          <h3>{t("Create a key for the bucket")}</h3>
          <p><Tx text={t("Back on the R2 page, under {details}: {manage} → {token}. Permissions: {rw}. Specify bucket(s): {only} → your bucket. Then {create}.")}
            em={{ details: "Account Details", rw: "Object Read & Write", only: "Apply to specific buckets only" }}
            strong={{ manage: "Manage API Tokens", token: "Create Account API token", create: "Create" }} /></p>
          <p><Tx text={t("The next page is shown only once. Copy the values under {s3}, not the Token value at the top.")}
            em={{ s3: "“Use the following credentials for S3 clients”" }} /></p>
          {@render keys(t("(“Default” under jurisdiction-specific endpoints, or S3 API on the R2 page)"))}
        </div>
      </li>
      <li>
        <div class="n">4</div>
        <div class="body">
          <h3>{t("Name your team")}</h3>
          {@render teamName()}
        </div>
      </li>
    </ol>
  {:else}
    <div class="plain">
      <p class="small muted"><Tx text={t("Create a bucket for DAWGit and a key that may read, write and delete objects in it. The storage must support conditional writes ({header}), which keeps two people from overwriting each other's versions; DAWGit checks this.")}
        code={{ header: "If-None-Match" }} /></p>
      <label for="s-bucket">{t("Bucket name")}</label>
      <input id="s-bucket" bind:value={bucket} autocomplete="off" spellcheck="false" />
      {@render keys("")}
      <div class="two">
        <div>
          <label for="s-region">{t("Region")}</label>
          <input id="s-region" bind:value={region} placeholder="auto" spellcheck="false" />
        </div>
        <div>
          <label for="s-folder">{t("Folder in the bucket")}</label>
          <input id="s-folder" bind:value={folder} placeholder="dawgit" spellcheck="false" />
        </div>
      </div>
      {@render teamName()}
    </div>
  {/if}

  {#if error}<p class="error">{error}</p>{/if}
  <div class="row actions">
    <span class="faint small">{t("DAWGit tests reading and writing before it saves anything.")}</span>
    <span class="spacer"></span>
    <button class="primary" disabled={!ready || busy} onclick={create}>{busy ? t("Checking…") : t("Check & create team")}</button>
  </div>
{/if}

<style>
  .intro { margin: 0 0 12px; }
  .provider { display: grid; grid-template-columns: 1fr 1fr; gap: 8px; margin-bottom: 12px; }
  .provider label { display: flex; flex-wrap: wrap; align-items: center; gap: 4px 8px; margin: 0; padding: 9px 12px;
    border: 1px solid var(--line); border-radius: 8px; cursor: pointer; color: var(--text); font-size: 13.5px; }
  .provider label.on { border-color: var(--accent); background: #1f3b35; }
  .provider input { width: auto; margin: 0; }
  .provider .faint { flex-basis: 100%; padding-left: 21px; font-size: 12px; }
  .guide { list-style: none; padding: 0; margin: 0; display: flex; flex-direction: column; gap: 10px; }
  .guide li, .plain { display: flex; gap: 12px; padding: 12px 14px; border: 1px solid var(--line); border-radius: 10px; background: var(--bg); }
  .plain { flex-direction: column; gap: 0; }
  .plain > p { margin: 0 0 4px; }
  .n { flex: none; width: 22px; height: 22px; border-radius: 50%; display: grid; place-items: center;
    background: #1f3b35; color: var(--accent); font-size: 12px; font-weight: 700; }
  .body { flex: 1; min-width: 0; }
  h3 { margin: 1px 0 4px; font-size: 14px; }
  .body p { margin: 0 0 8px; font-size: 13px; color: var(--muted); user-select: text; }
  .body p :global(strong), .body p :global(em) { color: var(--text); }
  label { margin-top: 8px; }
  .two { display: grid; grid-template-columns: 1fr 1fr; gap: 10px; }
  .actions { margin-top: 14px; align-items: center; }
  .error { color: var(--danger); margin: 12px 0 0; user-select: text; }
  .small { font-size: 12px; }
  .ok { color: var(--accent); font-weight: 600; margin-top: 0; }
  .done p { margin: 0 0 10px; }
  .step-h { margin: 0 0 10px; font-size: 15px; }
  .done .small { font-size: 12.5px; }
</style>
