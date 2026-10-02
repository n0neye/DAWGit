<script lang="ts">
  import { t } from "./i18n.svelte";
  import Tx from "./Tx.svelte";
  import { api, errorText, formatBytes } from "./api";
  import type { RulesDetail, RuleNode, RuleSuggestion } from "../../bindings/dawgit/desktop/models";
  import Modal from "./Modal.svelte";
  import FileIcon from "./FileIcon.svelte";
  import { toast } from "./notify.svelte";

  // A project's rules (.dawgit.yaml) without writing YAML: which tool's
  // preset applies to which folder, a switch per file and folder (tracked or
  // left out, and why), and the rules the switches wrote.
  let { root, onclose }: { root: string; onclose: () => void } = $props();

  const presetName = (p: string) => ({ ableton: "Ableton Live", unity: "Unity", unreal: "Unreal",
    design: t("Design files"), code: t("Code"), none: t("No preset") } as Record<string, string>)[p] ?? p;
  // What a preset leaves out, for people: "Library, Temp, Obj and 9 more".
  const leftOutText = (pats: string[]) => {
    const names = [...new Set(pats.map((p) => p.replace(/^\/|\/$/g, "")))];
    return names.length > 5 ? t("{names} and {n} more", { names: names.slice(0, 5).join(", "), n: names.length - 5 }) : names.join(", ");
  };
  // Why a file is tracked or not, for people.
  function reason(by: string): string {
    let m = by.match(/^preset (\S+): ignore "(.*)"$/);
    if (m) return t("left out by the {preset} preset ({pattern})", { preset: presetName(m[1]), pattern: m[2] });
    m = by.match(/^rule (\d+): (ignore|track) "(.*)"$/);
    if (m) return m[2] === "ignore" ? t("left out by your rule {n}", { n: m[1] }) : t("kept by your rule {n}", { n: m[1] });
    if (by.includes("always tracked")) return t("always tracked: it holds these rules");
    if (by.startsWith("no rule")) return "";
    return by;
  }

  let detail = $state<RulesDetail | null>(null);
  let folders = $state<Record<string, RuleNode[]>>({}); // loaded folders' contents ("" the project's)
  let open = $state<Record<string, boolean>>({});
  let busy = $state(false);

  async function loadFolder(rel: string) {
    try {
      folders[rel] = (await api.RulesFolder(root, rel)) ?? [];
    } catch (e) {
      toast(errorText(e), "error");
    }
  }
  async function reload() {
    try {
      detail = await api.ProjectRules(root);
    } catch (e) {
      toast(errorText(e), "error");
    }
    await Promise.all(Object.keys(folders).map(loadFolder));
  }
  $effect(() => {
    root;
    folders = {};
    open = {};
    reload().then(() => loadFolder(""));
  });

  // Every change is written to .dawgit.yaml right away.
  async function change(what: () => Promise<unknown>) {
    if (busy) return;
    busy = true;
    try {
      await what();
      await reload();
    } catch (e) {
      toast(errorText(e), "error");
    } finally {
      busy = false;
    }
  }
  const setPreset = (folder: string, preset: string) => change(() => api.SetPreset(root, folder, preset));
  const setTracked = (n: RuleNode, tracked: boolean) => change(() => api.SetTracked(root, n.path, n.dir, tracked));
  const removeRule = (i: number) => change(() => api.RemoveRule(root, i));

  function toggle(n: RuleNode) {
    open[n.path] = !open[n.path];
    if (open[n.path] && !folders[n.path]) loadFolder(n.path);
  }

  async function editText() {
    try {
      await api.OpenRules(root);
      toast(t("Save the file, then DAWGit follows the new rules"), "info");
    } catch (e) {
      toast(errorText(e), "error");
    }
  }
</script>

{#snippet suggestionRow(s: RuleSuggestion)}
  <div class="tool suggested">
    <div class="what">
      <Tx text={t(s.folder ? "{tool} project found in {folder}." : "{tool} project found in the project folder.")}
        strong={{ folder: `${s.folder}/`, tool: presetName(s.preset) }} />
      <span class="faint">· {s.leftOutBytes > 0 ? t("leaves out {what} ({size} here)", { what: leftOutText(s.leftOut), size: formatBytes(s.leftOutBytes) })
        : t("leaves out {what}", { what: leftOutText(s.leftOut) })}</span>
    </div>
    <button class="primary small" disabled={busy} onclick={() => setPreset(s.folder, s.preset)}>{t("Use {tool} rules", { tool: presetName(s.preset) })}</button>
    <button class="ghost small" disabled={busy} onclick={() => setPreset(s.folder, "none")}>{t("Not a project")}</button>
  </div>
{/snippet}

{#snippet tree(rel: string, depth: number)}
  {#each folders[rel] ?? [] as n (n.path)}
    {@const why = reason(n.by)}
    <div class="node" class:ignored={n.ignored} style:padding-left="{8 + depth * 18}px">
      {#if n.dir}
        <button class="chev" onclick={() => toggle(n)} aria-label={open[n.path] ? "Collapse" : "Expand"}>{open[n.path] ? "▾" : "▸"}</button>
      {:else}
        <span class="chev"></span>
      {/if}
      <FileIcon kind={n.dir ? "folder" : "other"} open={!!open[n.path]} faint={n.ignored} />
      <span class="name" title={n.path}>{n.name}</span>
      {#if n.preset}<span class="chip" title={t("presets: in .dawgit.yaml")}>{presetName(n.preset)}</span>{/if}
      <span class="why faint" title={n.by}>{why}</span>
      {#if !n.dir}<span class="size faint">{formatBytes(n.size)}</span>{/if}
      <input type="checkbox" class="switch" role="switch" checked={!n.ignored} disabled={busy || n.path === ".dawgit.yaml"}
        title={n.ignored ? t("Left out of versions: switch on to keep it") : t("In versions: switch off to leave it out")}
        onchange={(e) => setTracked(n, (e.currentTarget as HTMLInputElement).checked)} />
    </div>
    {#if n.dir && open[n.path]}
      {#if folders[n.path]}
        {@render tree(n.path, depth + 1)}
      {:else}
        <div class="node faint" style:padding-left="{26 + (depth + 1) * 18}px">{t("Loading…")}</div>
      {/if}
    {/if}
  {/each}
{/snippet}

<Modal title={t("Rules")} {onclose} width={860}>
  <p class="hint"><Tx text={t("Which files go into versions. Saved in the project's {file} right away; commit it to share the rules with the team. Files left out stay on everyone's disk.")} code={{ file: ".dawgit.yaml" }} /></p>

  {#if detail?.error}
    <div class="error-box">
      <div>⚠ {detail.error}</div>
      <button onclick={editText}>{t("Fix .dawgit.yaml")}</button>
    </div>
  {/if}

  {#if detail}
    <section>
      <h3>{t("Tools in this project")}</h3>
      <p class="hint">{t("A tool's preset leaves out what the tool makes again by itself (caches, backups) in its folder.")}</p>
      {#each detail.suggestions as s (s.folder + s.preset)}{@render suggestionRow(s)}{/each}
      {#each detail.presets as e (e.folder)}
        {@const opt = detail.options.find((o) => o.name === e.preset)}
        <div class="tool">
          <div class="what">
            <strong>{e.folder ? `${e.folder}/` : t("The project folder")}</strong>
            {#if e.found}<span class="chip found" title={t("DAWGit wrote this line from what it found")}>{t("found by DAWGit")}</span>{/if}
            {#if opt?.leftOut.length}<span class="faint">· {t("leaves out {what}", { what: leftOutText(opt.leftOut) })}</span>{/if}
          </div>
          <select value={e.preset} disabled={busy || !!detail.error}
            onchange={(ev) => setPreset(e.folder, (ev.currentTarget as HTMLSelectElement).value)}>
            {#each detail.options as o (o.name)}<option value={o.name}>{presetName(o.name)}</option>{/each}
            <option value="none">{presetName("none")}</option>
          </select>
        </div>
      {/each}
    </section>

    <section>
      <h3>{t("Files and folders")}</h3>
      <div class="tree">
        {@render tree("", 0)}
      </div>
    </section>

    <section>
      <h3>{t("Your rules")}</h3>
      {#each detail.rules as r, i (i)}
        <div class="rule">
          <span class="kind" class:keep={r.kind === "track"}>{r.kind === "ignore" ? "Leave out" : "Keep"}</span>
          <span class="mono">{r.pattern}</span>
          <button class="ghost small" disabled={busy} onclick={() => removeRule(i)} title={t("Remove this rule")}>✕</button>
        </div>
      {:else}
        <p class="faint">{t("None yet: the switches above add them. Later rules win.")}</p>
      {/each}
    </section>
  {:else}
    <p class="faint">{t("Reading the rules…")}</p>
  {/if}

  {#snippet footer()}
    <button class="ghost" onclick={() => api.OpenURL("https://github.com/n0neye/DAWGit/blob/main/docs/profiles.md")}>{t("Guide ↗")}</button>
    <button onclick={editText}>{t("Edit as text")}</button>
    <button class="primary" onclick={onclose}>{t("Done")}</button>
  {/snippet}
</Modal>

<style>
  section { margin-top: 18px; }
  h3 { margin: 0 0 6px; font-size: 12px; text-transform: uppercase; letter-spacing: .06em; color: var(--muted); }
  .hint { margin: 0 0 8px; color: var(--muted); font-size: 13px; }
  .mono { font-family: ui-monospace, monospace; font-size: 12px; }
  .error-box { display: flex; align-items: center; gap: 10px; padding: 10px 12px; margin: 8px 0; border-radius: 8px;
    background: #3a1f1f; border: 1px solid #6a3030; color: #f3c0bc; }
  .error-box > div { flex: 1; }
  .tool { display: flex; align-items: center; gap: 10px; padding: 8px 10px; border: 1px solid var(--line);
    border-radius: 8px; margin-bottom: 6px; background: var(--panel-2); }
  .tool.suggested { border-color: #5a4623; background: var(--warn-bg); color: #f0d9a8; }
  .tool .what { flex: 1; min-width: 0; }
  .chip { font-size: 11px; padding: 1px 7px; border-radius: 9px; background: var(--line); color: var(--muted); margin-left: 6px;
    white-space: nowrap; }
  .chip.found { background: #1f3a33; color: var(--accent); }
  select { background: var(--panel); color: var(--text); border: 1px solid var(--line); border-radius: 6px; padding: 4px 8px; }
  .tree { border: 1px solid var(--line); border-radius: 8px; max-height: 340px; overflow: auto; padding: 4px 0; }
  .node { display: flex; align-items: center; gap: 6px; padding: 3px 10px 3px 8px; font-size: 13px; }
  .node:hover { background: var(--panel-2); }
  .node.ignored .name { color: var(--faint); text-decoration: line-through; text-decoration-color: #555a63; }
  .chev { width: 16px; flex: none; background: none; border: 0; padding: 0; color: var(--muted); cursor: pointer; font-size: 11px; }
  .name { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .why { flex: 1; min-width: 0; font-size: 12px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .size { font-size: 12px; width: 70px; text-align: right; flex: none; }
  .switch { appearance: none; position: relative; width: 26px; height: 15px; margin: 0 0 0 8px; flex: none; cursor: pointer;
    border-radius: 8px; background: var(--line); transition: background .15s; }
  .switch::after { content: ""; position: absolute; top: 2px; left: 2px; width: 11px; height: 11px; border-radius: 50%;
    background: var(--muted); transition: transform .15s; }
  .switch:checked { background: var(--accent); }
  .switch:checked::after { transform: translateX(11px); background: #fff; }
  .switch:disabled { opacity: .5; cursor: default; }
  .rule { display: flex; align-items: center; gap: 10px; padding: 4px 0; }
  .rule .mono { flex: 1; }
  .kind { font-size: 11px; padding: 1px 8px; border-radius: 9px; background: #3a2626; color: #f3b4ae; }
  .kind.keep { background: #1f3a33; color: var(--accent); }
  button.small { padding: 3px 10px; font-size: 12px; }
</style>
