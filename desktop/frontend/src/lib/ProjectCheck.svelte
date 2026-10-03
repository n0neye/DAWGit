<script lang="ts">
  import { t, tn } from "./i18n.svelte";
  import { api, ago, errorText, formatBytes } from "./api";
  import type { Report } from "../../bindings/dawgit/internal/health/models";

  // A project's check: for Live Sets, the Live version they need, their
  // samples, plugins and packs, against this computer; and what a first
  // version uploads. mode: "added" (before the first version: what
  // teammates will need), "downloaded" (can this computer open it?), or
  // "check" (any time).
  let { root, mode = "check" }: { root: string; mode?: "added" | "downloaded" | "check" } = $props();

  let report = $state<Report | null>(null);
  let err = $state("");
  $effect(() => {
    report = null;
    err = "";
    api.ProjectCheck(root).then((r) => (report = r)).catch((e) => (err = errorText(e)));
  });

  let live = $derived(report?.live ?? null);
  let newest = $derived(live?.installed?.[0]);
  let pluginsMissing = $derived(live?.plugins.filter((p) => p.here === "no") ?? []);
  let packsMissing = $derived(live?.samples.packs.filter((p) => p.here === "no") ?? []);
  let showMissing = $state(false);
  let showPlugins = $state(false);
</script>

<div class="check">
  {#if err}
    <p class="muted">{t("Couldn't check the project:")} {err}</p>
  {:else if !report}
    <p class="muted">{t("Checking the project…")}</p>
  {:else}
    <ul>
      {#if live}
        <!-- Live version -->
        {#if live.opens === "older" && mode !== "added"}
          <li class="warn"><b>⚠</b> <span>{t("Saved with Live {need}; this computer has Live {have}. Live can't open sets saved by a newer version: update Live first.", { need: live.needs, have: newest?.version ?? "" })}</span></li>
        {:else if live.opens === "none" && mode !== "added"}
          <li class="warn"><b>⚠</b> <span>{t("Saved with Live {need}; no Ableton Live was found on this computer.", { need: live.needs })}</span></li>
        {:else if mode === "added"}
          <li class="info"><b>ⓘ</b> <span>{t("Saved with Live {need}: teammates need Live {need} or newer to open it.", { need: live.needs })}</span></li>
        {:else}
          <li class="ok"><b>✓</b> <span>{t("Saved with Live {need}; this computer has Live {have} {edition}.", { need: live.needs, have: newest?.version ?? "", edition: newest?.edition ?? "" })}</span></li>
        {/if}

        <!-- samples -->
        {#if live.samples.inProject}
          <li class="ok"><b>✓</b> <span>{tn(live.samples.inProject, "{n} sample in the project folder", "{n} samples in the project folder")}</span></li>
        {/if}
        {#if live.samples.external}
          <li class="ok"><b>✓</b> <span>{tn(live.samples.external, "{n} sample from elsewhere on this computer ({size}): DAWGit keeps it with the versions", "{n} samples from elsewhere on this computer ({size}): DAWGit keeps them with the versions", { size: formatBytes(live.samples.externalBytes) })}</span></li>
        {/if}
        {#if live.samples.packRefs}
          {@const packs = live.samples.packs.map((p) => p.name).join(", ")}
          {#if mode !== "added" && !packsMissing.length && live.samples.packs.every((p) => p.here === "yes")}
            <li class="ok"><b>✓</b> <span>{tn(live.samples.packRefs, "{n} sample from Live packs ({packs}), installed here", "{n} samples from Live packs ({packs}), installed here", { packs })}</span></li>
          {:else}
            <li class={packsMissing.length && mode !== "added" ? "warn" : "info"}><b>{packsMissing.length && mode !== "added" ? "⚠" : "ⓘ"}</b>
              <span>{tn(live.samples.packRefs, "{n} sample from Live packs ({packs}): packs aren't uploaded, everyone needs them installed", "{n} samples from Live packs ({packs}): packs aren't uploaded, everyone needs them installed", { packs })}
                {#if packsMissing.length && mode !== "added"}<br />{t("Not installed here: {packs}", { packs: packsMissing.map((p) => p.name).join(", ") })}{/if}
              </span></li>
          {/if}
        {/if}
        {#if live.samples.missing.length}
          <li class="warn"><b>⚠</b> <span>
            {tn(live.samples.missing.length, "{n} sample can't be found: the sets will play without it", "{n} samples can't be found: the sets will play without them")}
            <button class="link" onclick={() => (showMissing = !showMissing)}>{showMissing ? t("Hide") : t("Show")}</button>
            {#if showMissing}<span class="paths">{#each live.samples.missing as m}<code>{m}</code>{/each}</span>{/if}
            {#if mode !== "downloaded"}<br /><span class="muted">{t("In Live, File › Manage Files finds them; then save the set.")}</span>{/if}
          </span></li>
        {/if}

        <!-- plugins -->
        {#if !live.plugins.length}
          <li class="ok"><b>✓</b> <span>{t("No third-party plugins: Live's own devices only")}</span></li>
        {:else}
          {@const level = mode === "added" || !live.pluginsKnown ? "info" : pluginsMissing.length ? "warn" : "ok"}
          <li class={level}><b>{level === "warn" ? "⚠" : level === "ok" ? "✓" : "ⓘ"}</b> <span>
            {#if mode === "added"}
              {tn(live.plugins.length, "{n} third-party plugin. Teammates without it (or with another version) may not hear these devices the same: freeze or bounce those tracks if needed.", "{n} third-party plugins. Teammates without them (or with other versions) may not hear these devices the same: freeze or bounce those tracks if needed.")}
            {:else if pluginsMissing.length}
              {tn(pluginsMissing.length, "{n} plugin isn't on this computer: Live shows its devices as missing until it's installed", "{n} plugins aren't on this computer: Live shows their devices as missing until they're installed")}
            {:else if live.pluginsKnown}
              {tn(live.plugins.length, "The {n} third-party plugin is on this computer", "All {n} third-party plugins are on this computer")}
            {:else}
              {tn(live.plugins.length, "{n} third-party plugin (Live's plugin list couldn't be read: not checked)", "{n} third-party plugins (Live's plugin list couldn't be read: not checked)")}
            {/if}
            <button class="link" onclick={() => (showPlugins = !showPlugins)}>{showPlugins ? t("Hide") : t("Show")}</button>
            {#if showPlugins}
              <span class="plugins">
                {#each live.plugins as p}
                  <span class="plugin {p.here}">
                    <span class="mark">{p.here === "yes" ? "✓" : p.here === "no" ? "✗" : "?"}</span>
                    {p.name} <span class="faint">{p.format}{p.version ? ` · ${p.version}` : ""}{p.vendor ? ` · ${p.vendor}` : ""}</span>
                  </span>
                {/each}
              </span>
            {/if}
          </span></li>
        {/if}
      {/if}

      <!-- teammates, from the setups they share -->
      {#if live && report.teamSetups}
        {#each report.team ?? [] as m}
          {@const fine = m.opens === "yes" && !m.missingPlugins.length && !m.missingPacks.length}
          <li class={fine ? "ok" : "warn"}><b>{fine ? "✓" : "⚠"}</b> <span>
            {#if fine}
              {t("{name} can open it (Live {version})", { name: m.name, version: m.live })}
            {:else}
              <strong>{m.name}</strong>:
              {#if m.opens === "older"}{t("has Live {version}, older than {need}", { version: m.live, need: live.needs })}.{/if}
              {#if m.opens === "none"}{t("no Live found")}.{/if}
              {#if m.missingPlugins.length}{t("doesn't have {plugins}", { plugins: m.missingPlugins.join(", ") })}.{/if}
              {#if m.missingPacks.length}{t("doesn't have the packs {packs}", { packs: m.missingPacks.join(", ") })}.{/if}
            {/if}
            {#if m.otherVersions.length}
              <br /><span class="muted">{t("Other versions: {list}", { list: m.otherVersions.map((p) => t("{name} {theirs} (here {here})", { name: p.name, theirs: p.theirs, here: p.here })).join(", ") })}</span>
            {/if}
            <span class="faint small">· {t("setup from {when}", { when: ago(m.updated) })}</span>
          </span></li>
        {/each}
        {#if !report.team?.length}
          <li class="info"><b>ⓘ</b> <span>{t("No teammate shares their setup yet, so DAWGit can't tell whether they can open it. They can turn it on in the team's settings.")}</span></li>
        {/if}
        {#if !report.shareSetup}
          <li class="info"><b>ⓘ</b> <span>{t("Your setup isn't shared with the team: turn it on in the team's settings so teammates' checks include you.")}</span></li>
        {/if}
      {/if}

      <!-- upload -->
      {#if mode === "added"}
        <li class="info"><b>ⓘ</b> <span>{tn(report.files, "The first version uploads {n} file ({size})", "The first version uploads {n} files ({size})", { size: formatBytes(report.bytes) })}{report.ignored
          ? " · " + tn(report.ignored, "{n} file left out by the rules ({size})", "{n} files left out by the rules ({size})", { size: formatBytes(report.ignoredBytes) }) : ""}</span></li>
      {/if}
    </ul>
  {/if}
</div>

<style>
  .check { margin: 4px 0 8px; }
  ul { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 6px; font-size: 13px; }
  li { display: flex; gap: 8px; align-items: baseline; }
  li > b { flex: none; width: 14px; text-align: center; }
  li.ok > b { color: var(--accent); }
  li.info > b { color: var(--muted); }
  li.warn > b { color: var(--warn); }
  li.warn > span { color: #f0d9a8; }
  .link { border: none; background: transparent; color: var(--accent); padding: 0 0 0 4px; font-size: 12px; cursor: pointer; }
  .paths, .plugins { display: flex; flex-direction: column; gap: 2px; margin-top: 4px; }
  code { font-size: 11.5px; color: var(--muted); word-break: break-all; }
  .small { font-size: 11.5px; }
  .plugin { color: var(--text); }
  .plugin .mark { display: inline-block; width: 14px; }
  .plugin.yes .mark { color: var(--accent); }
  .plugin.no .mark { color: var(--warn); }
  .plugin.unknown .mark { color: var(--faint); }
</style>
