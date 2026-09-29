<script lang="ts">
  import { api, errorText, isConnectionCode, type TeamSummary } from "./api";

  // The address + token (or connection code) form, used by onboarding and
  // "Connect to another team".
  let { onconnected, submitLabel = "Connect" }: {
    onconnected: (t: TeamSummary) => void;
    submitLabel?: string;
  } = $props();

  let address = $state("");
  let token = $state("");
  let error = $state("");
  let busy = $state(false);
  let help = $state(false);

  async function connect() {
    busy = true;
    error = "";
    try {
      onconnected(await api.ConnectTeam(address.trim(), token.trim()));
    } catch (e) {
      error = errorText(e);
    } finally {
      busy = false;
    }
  }
</script>

<form onsubmit={(e) => { e.preventDefault(); connect(); }}>
  <label for="addr">Team server address or connection code</label>
  <input id="addr" bind:value={address} placeholder="http://192.168.0.11:7331  or  dawgit-s3:…" autocomplete="off" />
  {#if !isConnectionCode(address)}
    <label for="tok">Access token</label>
    <input id="tok" bind:value={token} autocomplete="off" />
  {/if}
  {#if error}<p class="error">{error}</p>{/if}
  <div class="row actions">
    <button type="button" class="ghost" onclick={() => (help = !help)}>
      {help ? "Hide" : "I'm the one setting up the team"}
    </button>
    <span class="spacer"></span>
    <button type="submit" class="primary" disabled={!address.trim() || busy}>{busy ? "Connecting…" : submitLabel}</button>
  </div>
  {#if help}
    <div class="help">
      <p><strong>Option 1 — a computer in your team runs the server.</strong> On that computer: Start menu →
        <em>DAWGit → DAWGit Team Server</em>. It shows the address and token for everyone (keep it running
        while you work together).</p>
      <p><strong>Option 2 — cloud storage, no computer has to stay on.</strong> Create an S3-compatible bucket
        (e.g. Cloudflare R2) and make a connection code for each member with
        <span class="mono">dawgit connection-code</span>. See the team setup guide for the steps.</p>
      <p class="faint">Then connect here with the address and token, or paste your connection code.</p>
    </div>
  {/if}
</form>

<style>
  .actions { margin-top: 16px; }
  .error { color: var(--danger); }
  .help {
    margin-top: 12px; padding: 10px 14px; border-radius: 8px; background: var(--bg);
    border: 1px solid var(--line); font-size: 13px; user-select: text;
  }
  .help p { margin: 6px 0; }
</style>
