<script lang="ts">
  import { api, errorText, type TeamSummary } from "./api";
  import CodeBox from "./CodeBox.svelte";

  // Create a team on Cloudflare R2 (or other S3-compatible storage), step by
  // step: the bucket, a key for it, then DAWGit checks it all and hands out
  // the connection code for teammates.
  let { onconnected, oncreated }: {
    onconnected: (t: TeamSummary) => void;
    oncreated?: () => void; // the team exists; the code is on screen
  } = $props();

  const R2 = "https://dash.cloudflare.com/?to=/:account/r2/overview";

  let endpoint = $state("");
  let bucket = $state("");
  let accessKey = $state("");
  let secretKey = $state("");
  let name = $state("");
  let folder = $state("");
  let region = $state("");
  let other = $state(false); // not R2: show folder and region
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
  <p class="muted intro">Your team's songs live in a storage bucket of your own, on Cloudflare R2. Nobody has to keep
    a computer running, and a small team usually stays within R2's free allowance. It takes about 5 minutes.</p>

  <ol class="guide">
    <li>
      <div class="n">1</div>
      <div class="body">
        <h3>Open Cloudflare R2</h3>
        <p>Sign up or log in, then open <strong>R2 Object Storage</strong>. The first time, Cloudflare asks you to
          activate R2 (it may ask for a payment method even for the free allowance).</p>
        <button onclick={() => api.OpenURL(R2)}>Open the Cloudflare dashboard ↗</button>
      </div>
    </li>
    <li>
      <div class="n">2</div>
      <div class="body">
        <h3>Create a bucket</h3>
        <p><strong>Create bucket</strong> → give it a name (e.g. <span class="mono">night-shift-dawgit</span>) →
          Location: <em>Automatic</em> → <strong>Create bucket</strong>. Use one bucket just for DAWGit.</p>
        <label for="s-bucket">Bucket name</label>
        <input id="s-bucket" bind:value={bucket} placeholder="night-shift-dawgit" autocomplete="off" spellcheck="false" />
      </div>
    </li>
    <li>
      <div class="n">3</div>
      <div class="body">
        <h3>Create a key for the bucket</h3>
        <p>Back on the R2 page: <strong>Manage API tokens</strong> → <strong>Create API token</strong> →
          Permissions: <em>Object Read &amp; Write</em> → <em>Apply to specific buckets only</em>: your bucket →
          <strong>Create</strong>. Copy the three values it shows (the secret is shown only once).</p>
        <label for="s-ep">Endpoint <span class="faint">(“S3 client” address, https://….r2.cloudflarestorage.com)</span></label>
        <input id="s-ep" bind:value={endpoint} placeholder="https://<account id>.r2.cloudflarestorage.com" autocomplete="off" spellcheck="false" />
        <label for="s-ak">Access Key ID</label>
        <input id="s-ak" bind:value={accessKey} autocomplete="off" spellcheck="false" />
        <label for="s-sk">Secret Access Key</label>
        <input id="s-sk" type="password" bind:value={secretKey} autocomplete="off" />
      </div>
    </li>
    <li>
      <div class="n">4</div>
      <div class="body">
        <h3>Name your team</h3>
        <label for="s-name">Team name <span class="faint">(everyone sees it)</span></label>
        <input id="s-name" bind:value={name} placeholder="e.g. Night Shift" />
        {#if other}
          <div class="two">
            <div>
              <label for="s-folder">Folder in the bucket</label>
              <input id="s-folder" bind:value={folder} placeholder="dawgit" spellcheck="false" />
            </div>
            <div>
              <label for="s-region">Region</label>
              <input id="s-region" bind:value={region} placeholder="auto" spellcheck="false" />
            </div>
          </div>
        {:else}
          <button class="link" onclick={() => (other = true)}>Using other S3-compatible storage?</button>
        {/if}
      </div>
    </li>
  </ol>

  {#if error}<p class="error">{error}</p>{/if}
  <div class="row actions">
    <span class="faint small">DAWGit tests reading and writing before it saves anything.</span>
    <span class="spacer"></span>
    <button class="primary" disabled={!ready || busy} onclick={create}>{busy ? "Checking…" : "Check & create team"}</button>
  </div>
{/if}

<style>
  .intro { margin: 0 0 14px; }
  .guide { list-style: none; padding: 0; margin: 0; display: flex; flex-direction: column; gap: 10px; }
  .guide li { display: flex; gap: 12px; padding: 12px 14px; border: 1px solid var(--line); border-radius: 10px; background: var(--bg); }
  .n { flex: none; width: 22px; height: 22px; border-radius: 50%; display: grid; place-items: center;
    background: #1f3b35; color: var(--accent); font-size: 12px; font-weight: 700; }
  .body { flex: 1; min-width: 0; }
  h3 { margin: 1px 0 4px; font-size: 14px; }
  .body p { margin: 0 0 8px; font-size: 13px; color: var(--muted); user-select: text; }
  .body p strong, .body p em { color: var(--text); }
  label { margin-top: 8px; }
  .two { display: grid; grid-template-columns: 1fr 1fr; gap: 10px; }
  .link { border: none; background: none; padding: 0; margin-top: 8px; color: var(--muted); text-decoration: underline; font-size: 12.5px; }
  .actions { margin-top: 14px; align-items: center; }
  .error { color: var(--danger); margin: 12px 0 0; user-select: text; }
  .small { font-size: 12px; }
  .ok { color: var(--accent); font-weight: 600; margin-top: 0; }
  .done p { margin: 0 0 10px; }
</style>
