<script lang="ts">
  import { api } from "./api";
  import Modal from "./Modal.svelte";
  import { languages, chosenLanguage, setLanguage, t } from "./i18n.svelte";

  // DAWGit's own settings (not a project's or a team's): language, starting
  // with Windows, updates, where downloads go, and help.
  let { version, edition, autostart, autoUpdate, downloadDir, update, checking, onautostart, onautoupdate, oncheck,
    ondownloaddir, onclose }: {
    version: string;
    edition: string;
    autostart: boolean;
    autoUpdate: boolean;
    downloadDir: string;
    update: { version: string } | null; // a newer release
    checking: boolean;
    onautostart: (on: boolean) => void;
    onautoupdate: (on: boolean) => void;
    oncheck: () => void;
    ondownloaddir: () => void;
    onclose: () => void;
  } = $props();

  let lang = $state(chosenLanguage());
  async function pickLanguage(code: string) {
    lang = code;
    await setLanguage(code);
  }

  const repo = "https://github.com/n0neye/DAWGit";
  // A new issue with what helps to look into it filled in (nothing about the
  // user or their projects).
  function reportIssue() {
    const body = [
      `**DAWGit** ${version}${edition ? ` ${edition}` : ""}`,
      `**System** ${navigator.userAgent.match(/Windows NT [\d.]+|Mac OS X [\d_]+/)?.[0] ?? navigator.platform}`,
      "",
      "**What happened**",
      "",
      "**What you expected**",
      "",
      "**Steps to see it again**",
      "1. ",
    ].join("\n");
    api.OpenURL(`${repo}/issues/new?body=${encodeURIComponent(body)}`);
  }
</script>

<Modal title={t("DAWGit settings")} {onclose} width={560}>
  <section>
    <h3>{t("General")}</h3>
    <div class="line">
      <span class="label">{t("Language")}</span>
      <select value={lang} onchange={(e) => pickLanguage((e.currentTarget as HTMLSelectElement).value)}>
        <option value="">{t("Same as the system")}</option>
        {#each languages as l (l.code)}<option value={l.code}>{l.name}</option>{/each}
      </select>
    </div>
    <label class="check">
      <input type="checkbox" checked={autostart} onchange={(e) => onautostart(e.currentTarget.checked)} />
      <span>{t("Start with Windows")}
        <span class="hint">{t("Keeps DAWGit in the tray, so you hear about new versions from your team.")}</span></span>
    </label>
    <div class="line">
      <span class="label">{t("Downloads go to")}</span>
      <span class="mono path" title={downloadDir}>{downloadDir || t("a folder you choose")}</span>
      <button class="small" onclick={ondownloaddir}>{t("Change…")}</button>
    </div>
  </section>

  <section>
    <h3>{t("Updates")}</h3>
    <div class="line">
      <span class="label">{t("Version")}</span>
      <span>DAWGit{edition ? ` ${edition}` : ""} {version}</span>
      <span class="spacer"></span>
      {#if update}
        <span class="new">{t("{version} is available", { version: update.version })}</span>
      {/if}
      <button class="small" onclick={oncheck} disabled={checking}>{checking ? t("Checking…") : t("Check for updates")}</button>
    </div>
    <label class="check">
      <input type="checkbox" checked={autoUpdate} onchange={(e) => onautoupdate(e.currentTarget.checked)} />
      <span>{t("Install updates automatically")}
        <span class="hint">{t("When DAWGit is in the tray and idle, or when it quits.")}</span></span>
    </label>
  </section>

  <section>
    <h3>{t("Help")}</h3>
    <div class="links">
      <button onclick={() => api.OpenURL(repo)}>GitHub ↗</button>
      <button onclick={() => api.OpenURL(`${repo}#getting-started`)}>{t("Guide")} ↗</button>
      <button onclick={() => api.OpenURL(`${repo}/releases`)}>{t("What's new")} ↗</button>
      <button onclick={reportIssue}>{t("Report an issue")} ↗</button>
    </div>
  </section>

  {#snippet footer()}
    <button class="primary" onclick={onclose}>{t("Done")}</button>
  {/snippet}
</Modal>

<style>
  section { padding: 12px 0; border-bottom: 1px solid var(--line); }
  section:last-of-type { border-bottom: 0; }
  h3 { margin: 0 0 10px; font-size: 11px; text-transform: uppercase; letter-spacing: .08em; color: var(--faint); }
  .line { display: flex; align-items: center; gap: 10px; margin: 8px 0; min-height: 28px; }
  .label { width: 130px; flex: none; color: var(--muted); font-size: 13px; }
  .path { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .spacer { flex: 1; }
  .new { color: var(--accent); font-size: 12.5px; }
  select { background: var(--panel); color: var(--text); border: 1px solid var(--line); border-radius: 6px;
    padding: 5px 8px; min-width: 200px; }
  label.check { display: flex; align-items: flex-start; gap: 10px; margin: 10px 0; color: var(--text); font-size: 13.5px;
    cursor: pointer; }
  label.check input { margin-top: 3px; width: auto; flex: none; }
  label.check > span { flex: 1; }
  .hint { display: block; color: var(--faint); font-size: 12px; margin-top: 2px; }
  .links { display: flex; flex-wrap: wrap; gap: 8px; }
  button.small { padding: 4px 10px; font-size: 12.5px; }
</style>
