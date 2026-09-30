<script lang="ts">
  import { api, errorText, isConnectionCode, type TeamSummary } from "./api";

  // Join a team with its connection code (or a team server's address and
  // token), in onboarding and "Join/Create a team".
  let { onconnected, submitLabel = "Connect" }: {
    onconnected: (t: TeamSummary) => void;
    submitLabel?: string;
  } = $props();

  let address = $state("");
  let token = $state("");
  let error = $state("");
  let busy = $state(false);

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
  <label for="addr">Connection code</label>
  <input id="addr" bind:value={address} placeholder="dawgit-s3:…" autocomplete="off" spellcheck="false" />
  <!-- A team server's address still works (with its token). -->
  {#if address.trim() && !isConnectionCode(address)}
    <label for="tok">Access token</label>
    <input id="tok" bind:value={token} autocomplete="off" />
  {/if}
  {#if error}<p class="error">{error}</p>{/if}
  <div class="row actions">
    <span class="spacer"></span>
    <button type="submit" class="primary" disabled={!address.trim() || busy}>{busy ? "Connecting…" : submitLabel}</button>
  </div>
</form>

<style>
  .actions { margin-top: 16px; }
  .error { color: var(--danger); user-select: text; }
</style>
