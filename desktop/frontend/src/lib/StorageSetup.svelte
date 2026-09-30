<script lang="ts">
  import { api, errorText, type TeamSummary } from "./api";
  import CodeBox from "./CodeBox.svelte";

  // Create a team on S3-compatible storage. Cloudflare R2 comes with a step
  // by step guide; any other S3-compatible storage takes the same fields.
  // DAWGit checks it all, then hands out the connection code for teammates.
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
  <label for="s-ak">Access Key ID</label>
  <input id="s-ak" bind:value={accessKey} autocomplete="off" spellcheck="false" />
  <label for="s-sk">Secret Access Key</label>
  <input id="s-sk" type="password" bind:value={secretKey} autocomplete="off" />
  <label for="s-ep">Endpoint {#if hint}<span class="faint">{hint}</span>{/if}</label>
  <input id="s-ep" bind:value={endpoint} autocomplete="off" spellcheck="false"
    placeholder={provider === "r2" ? "https://<account id>.r2.cloudflarestorage.com" : "https://s3.<region>.amazonaws.com"} />
{/snippet}

{#snippet teamName()}
  <label for="s-name">Team name <span class="faint">(everyone sees it)</span></label>
  <input id="s-name" bind:value={name} placeholder="e.g. Night Shift" />
{/snippet}

{#if created}
  <div class="done">
    <p class="ok">✓ Storage checked — “{created.team.name}” is ready.</p>
    {#if created.team.name !== name.trim()}
      <p class="faint small">This bucket already holds the team “{created.team.name}”, so you joined it.</p>
    {/if}
    <p>Send this <strong>connection code</strong> to each teammate. They choose <em>Join a team</em> and paste it.</p>
    <CodeBox code={created.code} />
    <p class="faint small">The code contains the storage key: send it privately (a direct message, not a public
      channel). You can copy it again later from the ⚙ next to the team in the Team menu.</p>
    <div class="row actions">
      <span class="spacer"></span>
      <button class="primary" onclick={() => onconnected(created!.team)}>Continue</button>
    </div>
  </div>
{:else}
  <p class="muted intro">Your team's songs live in a storage bucket of your own, so nobody has to keep a computer
    running. Any S3-compatible storage works; Cloudflare R2 is the easiest start (no download fees, and a small
    team usually stays within its free allowance).</p>

  <div class="provider" role="radiogroup" aria-label="Storage">
    <label class:on={provider === "r2"}><input type="radio" bind:group={provider} value="r2" /> Cloudflare R2
      <span class="faint">guided, about 5 minutes</span></label>
    <label class:on={provider === "s3"}><input type="radio" bind:group={provider} value="s3" /> Other S3-compatible
      <span class="faint">Amazon S3, MinIO, …</span></label>
  </div>

  {#if provider === "r2"}
    <ol class="guide">
      <li>
        <div class="n">1</div>
        <div class="body">
          <h3>Open R2 in Cloudflare</h3>
          <p>Sign up or log in, then in the sidebar open <strong>Storage &amp; databases → R2 Object Storage</strong>.
            The first time, Cloudflare asks you to activate R2 (it may ask for a payment method even for the free
            allowance).</p>
          <button onclick={() => api.OpenURL(DASHBOARD)}>Open the Cloudflare dashboard ↗</button>
        </div>
      </li>
      <li>
        <div class="n">2</div>
        <div class="body">
          <h3>Create a bucket</h3>
          <p><strong>Create bucket</strong> → a name (e.g. <span class="mono">night-shift-dawgit</span>) → keep
            Location <em>Automatic</em> and Storage Class <em>Standard</em> → <strong>Create bucket</strong>.
            Use a bucket just for DAWGit.</p>
          <label for="s-bucket">Bucket name</label>
          <input id="s-bucket" bind:value={bucket} placeholder="night-shift-dawgit" autocomplete="off" spellcheck="false" />
        </div>
      </li>
      <li>
        <div class="n">3</div>
        <div class="body">
          <h3>Create a key for the bucket</h3>
          <p>Back on the R2 page, under <em>Account Details</em>: <strong>Manage API Tokens</strong> →
            <strong>Create Account API token</strong>. Permissions: <em>Object Read &amp; Write</em>. Specify
            bucket(s): <em>Apply to specific buckets only</em> → your bucket. Then <strong>Create</strong>.</p>
          <p>The next page is shown only once. Copy the values under <em>“Use the following credentials for S3
            clients”</em>, not the Token value at the top.</p>
          {@render keys("(“Default” under jurisdiction-specific endpoints, or S3 API on the R2 page)")}
        </div>
      </li>
      <li>
        <div class="n">4</div>
        <div class="body">
          <h3>Name your team</h3>
          {@render teamName()}
        </div>
      </li>
    </ol>
  {:else}
    <div class="plain">
      <p class="small muted">Create a bucket for DAWGit and a key that may read, write and delete objects in it. The
        storage must support conditional writes (<span class="mono">If-None-Match</span>), which keeps two people
        from overwriting each other's versions; DAWGit checks this.</p>
      <label for="s-bucket">Bucket name</label>
      <input id="s-bucket" bind:value={bucket} autocomplete="off" spellcheck="false" />
      {@render keys("")}
      <div class="two">
        <div>
          <label for="s-region">Region</label>
          <input id="s-region" bind:value={region} placeholder="auto" spellcheck="false" />
        </div>
        <div>
          <label for="s-folder">Folder in the bucket</label>
          <input id="s-folder" bind:value={folder} placeholder="dawgit" spellcheck="false" />
        </div>
      </div>
      {@render teamName()}
    </div>
  {/if}

  {#if error}<p class="error">{error}</p>{/if}
  <div class="row actions">
    <span class="faint small">DAWGit tests reading and writing before it saves anything.</span>
    <span class="spacer"></span>
    <button class="primary" disabled={!ready || busy} onclick={create}>{busy ? "Checking…" : "Check & create team"}</button>
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
  .body p strong, .body p em { color: var(--text); }
  label { margin-top: 8px; }
  .two { display: grid; grid-template-columns: 1fr 1fr; gap: 10px; }
  .actions { margin-top: 14px; align-items: center; }
  .error { color: var(--danger); margin: 12px 0 0; user-select: text; }
  .small { font-size: 12px; }
  .ok { color: var(--accent); font-weight: 600; margin-top: 0; }
  .done p { margin: 0 0 10px; }
</style>
