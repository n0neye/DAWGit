<script lang="ts">
  import { t, tn } from "./i18n.svelte";
  import { api, ago, errorText, formatBytes } from "./api";
  import CheckIcon from "./CheckIcon.svelte";
  import type { Report } from "../../bindings/dawgit/internal/health/models";

  // A project's check, at a glance: tiles for the Live version, samples,
  // plugins, teammates and (before the first version) the upload, each
  // with a mark (✓ fine, ! needs a look); what needs doing is listed below
  // in a line each; the long explanations are in the tiles' tooltips.
  // mode: "added" (before the first version: what teammates will need),
  // "downloaded" (can this computer open it?), or "check" (any time).
  let { root, mode = "check", onrestore }: {
    root: string; mode?: "added" | "downloaded" | "check";
    onrestore?: () => void; // bring back the missing samples DAWGit has copies of
  } = $props();

  let report = $state<Report | null>(null);
  let err = $state("");
  $effect(() => {
    report = null;
    err = "";
    api.ProjectCheck(root).then((r) => (report = r)).catch((e) => (err = errorText(e)));
  });

  let open = $state(""); // the tile whose list is open
  const toggle = (k: string) => (open = open === k ? "" : k);

  let live = $derived(report?.live ?? null);
  let here = $derived(mode !== "added"); // judged against this computer
  let newest = $derived(live?.installed?.[0]);
  let s = $derived(live?.samples);
  let pluginsMissing = $derived(live?.plugins.filter((p) => p.here === "no") ?? []);
  let packsMissing = $derived(s?.packs.filter((p) => p.here === "no") ?? []);
  let sampleCount = $derived(s ? s.inProject + s.external + s.packRefs + s.missing.length : 0);
  let team = $derived(report?.team ?? []);
  const teamFine = (m: (typeof team)[number]) => m.opens === "yes" && !m.missingPlugins.length && !m.missingPacks.length;

  // note: worth knowing, nothing to fix first (old projects often miss a
  // sample); warn: needs doing before it plays right here.
  type Mark = "ok" | "warn" | "note" | "info";
  let liveMark = $derived<Mark>(!here ? "info" : live?.opens === "yes" ? "ok" : "warn");
  let sampleMark = $derived<Mark>(here && packsMissing.length ? "warn" : s?.missing.length ? "note" : "ok");
  let pluginMark = $derived<Mark>(!live?.plugins.length ? "ok" : !here || !live.pluginsKnown ? "info" : pluginsMissing.length ? "warn" : "ok");
  let teamMark = $derived<Mark>(!team.length ? "info" : team.every(teamFine) ? "ok" : "warn");
  const initials = (n: string) => n.split(/\s+/).map((w) => w[0] ?? "").join("").slice(0, 2).toUpperCase();
</script>

{#snippet mark(m: Mark)}
  {#if m === "ok"}<span class="badge ok"><CheckIcon name="check" size={11} /></span>
  {:else if m === "warn"}<span class="badge warn">!</span>
  {:else if m === "note"}<span class="badge note">!</span>{/if}
{/snippet}

<div class="check">
  {#if err}
    <p class="muted">{t("Couldn't check the project:")} {err}</p>
  {:else if !report}
    <p class="muted">{t("Checking the project…")}</p>
  {:else}
    <div class="tiles">
      {#if live}
        <div class="tile {liveMark}" title={here ? (live.opens === "yes"
            ? t("Saved with Live {need}; this computer has Live {have} {edition}.", { need: live.needs, have: newest?.version ?? "", edition: newest?.edition ?? "" })
            : live.opens === "older"
            ? t("Saved with Live {need}; this computer has Live {have}. Live can't open sets saved by a newer version: update Live first.", { need: live.needs, have: newest?.version ?? "" })
            : t("Saved with Live {need}; no Ableton Live was found on this computer.", { need: live.needs }))
          : t("Saved with Live {need}: teammates need Live {need} or newer to open it.", { need: live.needs })}>
          {@render mark(liveMark)}
          <CheckIcon name="live" size={22} />
          <span class="value">{live.needs}</span>
          <span class="label">Live</span>
          {#if here && newest}<span class="sub">{t("here {version}", { version: newest.version })}</span>{/if}
        </div>

        <button class="tile {sampleMark}" onclick={() => toggle("samples")} class:on={open === "samples"}
          title={t("Samples the sets use: in the project, elsewhere (kept by DAWGit), from Live packs (not uploaded), missing")}>
          {@render mark(sampleMark)}
          <CheckIcon name="wave" size={22} />
          <span class="value">{sampleCount}</span>
          <span class="label">{t("Samples")}</span>
          <span class="chips">
            {#if s?.inProject}<span title={tn(s.inProject, "{n} sample in the project folder", "{n} samples in the project folder")}><CheckIcon name="folder" size={12} />{s.inProject}</span>{/if}
            {#if s?.external}<span title={tn(s.external, "{n} sample from elsewhere on this computer ({size}): DAWGit keeps it with the versions", "{n} samples from elsewhere on this computer ({size}): DAWGit keeps them with the versions", { size: formatBytes(s.externalBytes) })}><CheckIcon name="link" size={12} />{s.external}</span>{/if}
            {#if s?.packRefs}<span title={s.packs.map((p) => p.name).join(", ")}><CheckIcon name="package" size={12} />{s.packRefs}</span>{/if}
            {#if s?.missing.length}<span class="note" title={tn(s.missing.length, "{n} sample can't be found: the sets will play without it", "{n} samples can't be found: the sets will play without them")}><CheckIcon name="x" size={12} />{s.missing.length}</span>{/if}
          </span>
        </button>

        <button class="tile {pluginMark}" onclick={() => toggle("plugins")} class:on={open === "plugins"} disabled={!live.plugins.length}
          title={live.plugins.length
            ? tn(live.plugins.length, "{n} third-party plugin. Teammates without it (or with another version) may not hear these devices the same: freeze or bounce those tracks if needed.", "{n} third-party plugins. Teammates without them (or with other versions) may not hear these devices the same: freeze or bounce those tracks if needed.")
            : t("No third-party plugins: Live's own devices only")}>
          {@render mark(pluginMark)}
          <CheckIcon name="plug" size={22} />
          <span class="value">{live.plugins.length}</span>
          <span class="label">{t("Plugins")}</span>
          {#if here && live.plugins.length}
            <span class="sub">{!live.pluginsKnown ? "?" : pluginsMissing.length ? t("{n} missing here", { n: pluginsMissing.length }) : t("all here")}</span>
          {/if}
        </button>

        {#if report.teamSetups}
          <button class="tile {teamMark}" onclick={() => toggle("team")} class:on={open === "team"} disabled={!team.length}
            title={team.length ? t("Teammates who share their setup: can they open it?") : t("No teammate shares their setup yet, so DAWGit can't tell whether they can open it. They can turn it on in the team's settings.")}>
            {@render mark(teamMark)}
            <CheckIcon name="users" size={22} />
            {#if team.length}
              <span class="faces">
                {#each team.slice(0, 4) as m}<span class="face" class:bad={!teamFine(m)} title={m.name}>{initials(m.name)}</span>{/each}
              </span>
            {:else}<span class="value">–</span>{/if}
            <span class="label">{t("Teammates")}</span>
          </button>
        {/if}
      {/if}

      {#if mode === "added"}
        <div class="tile info" title={report.ignored ? tn(report.ignored, "{n} file left out by the rules ({size})", "{n} files left out by the rules ({size})", { size: formatBytes(report.ignoredBytes) }) : ""}>
          <CheckIcon name="upload" size={22} />
          <span class="value">{formatBytes(report.bytes)}</span>
          <span class="label">{t("First upload")}</span>
          <span class="sub">{tn(report.files, "{n} file", "{n} files")}</span>
        </div>
      {/if}
    </div>

    <!-- what needs a look, a line each -->
    <ul class="issues">
      {#if live && here && live.opens === "older"}
        <li><CheckIcon name="alert" size={14} />{t("Update Live to {need} or newer", { need: live.needs })}</li>
      {:else if live && here && live.opens === "none"}
        <li><CheckIcon name="alert" size={14} />{t("Ableton Live not found on this computer")}</li>
      {/if}
      {#if s?.missing.length}
        <li class="note"><CheckIcon name="alert" size={14} />{mode === "downloaded"
          ? tn(s.missing.length, "{n} sample missing in the shared version. Whoever saved it may still have it: they can find it in Live and share again.", "{n} samples missing in the shared version. Whoever saved them may still have them: they can find them in Live and share again.")
          : mode === "added"
          ? tn(s.missing.length, "{n} sample missing. Best found in Live before you share (File › Manage Files), so the team hears the same.", "{n} samples missing. Best found in Live before you share (File › Manage Files), so the team hears the same.")
          : tn(s.missing.length, "{n} sample missing. Find it in Live (File › Manage Files) before your next share.", "{n} samples missing. Find them in Live (File › Manage Files) before your next share.")}
          {#if onrestore && s.restorable}<button class="link strong" onclick={onrestore}>{tn(s.restorable, "Restore it from DAWGit", "Restore {n} from DAWGit")}</button>{/if}
          <button class="link" onclick={() => toggle("missing")}>{open === "missing" ? t("Hide") : t("Show")}</button></li>
      {/if}
      {#if here && pluginsMissing.length}
        <li><CheckIcon name="alert" size={14} />{t("Not on this computer: {plugins}", { plugins: pluginsMissing.map((p) => p.name).join(", ") })}</li>
      {/if}
      {#if here && packsMissing.length}
        <li><CheckIcon name="alert" size={14} />{t("Not installed here: {packs}", { packs: packsMissing.map((p) => p.name).join(", ") })}</li>
      {/if}
      {#each team.filter((m) => !teamFine(m)) as m}
        <li><CheckIcon name="alert" size={14} /><strong>{m.name}</strong>:
          {[m.opens === "older" ? `Live ${m.live}` : m.opens === "none" ? t("no Live found") : "",
            ...m.missingPlugins, ...m.missingPacks].filter(Boolean).join(", ")}</li>
      {/each}
      {#if report.teamSetups && !report.shareSetup}
        <li class="soft"><CheckIcon name="users" size={14} />{t("Your setup isn't shared with the team")}</li>
      {/if}
    </ul>

    <!-- a tile's list -->
    {#if open === "missing" && s}
      <div class="list">{#each s.missing as m}<code>{m}</code>{/each}
        <span class="muted">{t("In Live, File › Manage Files finds them; then save the set.")}</span></div>
    {:else if open === "samples" && s}
      <div class="list">
        {#if s.inProject}<span><CheckIcon name="folder" size={13} /> {tn(s.inProject, "{n} sample in the project folder", "{n} samples in the project folder")}</span>{/if}
        {#if s.external}<span><CheckIcon name="link" size={13} /> {tn(s.external, "{n} sample from elsewhere on this computer ({size}): DAWGit keeps it with the versions", "{n} samples from elsewhere on this computer ({size}): DAWGit keeps them with the versions", { size: formatBytes(s.externalBytes) })}</span>{/if}
        {#each s.packs as p}<span class="pack {here ? p.here : ''}"><CheckIcon name="package" size={13} /> {p.name}</span>{/each}
      </div>
    {:else if open === "plugins" && live}
      <div class="list">
        {#each live.plugins as p}
          <span class="plugin {here ? p.here : ''}">
            <CheckIcon name={!here ? "plug" : p.here === "yes" ? "check" : p.here === "no" ? "x" : "question"} size={13} />
            {p.name} <span class="faint">{p.format}{p.version ? ` · ${p.version}` : ""}{p.vendor ? ` · ${p.vendor}` : ""}</span>
          </span>
        {/each}
      </div>
    {:else if open === "team"}
      <div class="list">
        {#each team as m}
          <span class="plugin {teamFine(m) ? 'yes' : 'no'}">
            <CheckIcon name={teamFine(m) ? "check" : "alert"} size={13} />
            <strong>{m.name}</strong> <span class="faint">Live {m.live} · {t("setup from {when}", { when: ago(m.updated) })}</span>
            {#if m.otherVersions.length}<span class="faint">· {m.otherVersions.map((p) => `${p.name} ${p.theirs}`).join(", ")}</span>{/if}
          </span>
        {/each}
      </div>
    {/if}
  {/if}
</div>

<style>
  .check { margin: 6px 0 10px; --note: #f0d65c; }
  .tiles { display: grid; grid-template-columns: repeat(auto-fit, minmax(80px, 1fr)); gap: 6px; }
  .tile { position: relative; display: flex; flex-direction: column; align-items: center; gap: 2px; padding: 10px 4px 8px; min-width: 0;
    border: 1px solid var(--line); border-radius: 10px; background: var(--panel); color: var(--muted); font: inherit; text-align: center; }
  button.tile { cursor: pointer; }
  button.tile:disabled { cursor: default; }
  button.tile:not(:disabled):hover, .tile.on { border-color: var(--muted); }
  .tile.warn { border-color: #6b5426; background: var(--warn-bg); color: var(--warn); }
  .value { font-size: 17px; font-weight: 600; color: var(--text); line-height: 1.2; }
  .label { font-size: 11.5px; color: var(--muted); }
  .sub { font-size: 11px; color: var(--faint); }
  .tile.warn .sub { color: var(--warn); }
  .badge { position: absolute; top: 6px; right: 6px; width: 16px; height: 16px; border-radius: 50%; display: flex;
    align-items: center; justify-content: center; font-size: 11px; font-weight: 700; }
  .badge.ok { background: rgba(63, 195, 169, .18); color: var(--accent); }
  .badge.warn { background: var(--warn); color: #1b1407; }
  .badge.note { background: rgba(240, 214, 92, .2); color: var(--note); }
  .chips { display: flex; gap: 6px; font-size: 11px; color: var(--faint); }
  .chips span { display: inline-flex; align-items: center; gap: 2px; }
  .chips .note { color: var(--note); }
  .faces { display: flex; margin: 1px 0 2px; }
  .face { width: 22px; height: 22px; border-radius: 50%; margin-left: -4px; border: 2px solid var(--panel); background: #2f5e55;
    color: #d9f3ec; font-size: 9.5px; font-weight: 700; display: flex; align-items: center; justify-content: center; }
  .face:first-child { margin-left: 0; }
  .face.bad { background: #6b5426; color: #f6e2b8; }
  .issues { list-style: none; margin: 10px 0 0; padding: 0; display: flex; flex-direction: column; gap: 4px; font-size: 12.5px; }
  .issues li { display: flex; align-items: center; gap: 6px; color: #f0d9a8; }
  .issues li :global(svg) { color: var(--warn); flex: none; }
  .issues li.soft, .issues li.soft :global(svg) { color: var(--faint); }
  .issues li.note { color: var(--text); }
  .issues li.note :global(svg) { color: var(--note); }
  .link.strong { font-weight: 600; }
  .issues .link { white-space: nowrap; flex: none; }
  .link { border: none; background: transparent; color: var(--accent); padding: 0 0 0 4px; font-size: 12px; cursor: pointer; }
  .list { display: flex; flex-direction: column; gap: 3px; margin-top: 8px; padding: 8px 10px; border-radius: 8px;
    background: var(--bg); border: 1px solid var(--line); font-size: 12.5px; }
  .list span { display: flex; align-items: center; gap: 6px; }
  code { font-size: 11.5px; color: var(--muted); word-break: break-all; }
  .plugin.yes :global(svg), .pack.yes :global(svg) { color: var(--accent); }
  .plugin.no :global(svg), .pack.no :global(svg) { color: var(--warn); }
  .plugin.unknown :global(svg) { color: var(--faint); }
</style>
