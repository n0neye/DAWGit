<script lang="ts">
  import { t } from "./i18n.svelte";
  import { api, ago, errorText, type State } from "./api";
  import { toast } from "./notify.svelte";
  import PreuploadIcon from "./PreuploadIcon.svelte";
  import { preuploads } from "./preupload.svelte";

  // The top of a project's page: its name, the branch menu (switch, merge,
  // new branch), the team it's shared with, and opening it in its tool.
  let { st, refreshing, onswitch, onmerge, onnewbranch, onsettings, oncheck, onrefresh }: {
    st: State;
    refreshing: boolean;
    onswitch: (branch: string) => void;
    onmerge: (branch: string) => void; // into the current one
    onnewbranch: () => void;
    onsettings: () => void;
    oncheck: () => void;
    onrefresh: () => void;
  } = $props();

  let branchMenu = $state(false);
  let setMenu = $state(false);
  let isLive = $derived(st.tool === "Ableton Live");
  const label = (rel: string) => (rel === "." ? st.name ?? t("the project") : rel);
  const open = (rel: string) => api.OpenInTool(st.root, rel).catch((e) => toast(errorText(e), "error"));
  let toolName = $derived(isLive ? "Live" : st.tool ? t(st.tool) : t("app"));
</script>

<svelte:window onclick={(e) => {
  const el = e.target as HTMLElement;
  if (branchMenu && !el.closest(".branch-wrap")) branchMenu = false;
  if (setMenu && !el.closest(".open-wrap")) setMenu = false;
}} />

<header>
  <div class="title">
    <h1>{st.name}</h1>
    <div class="sub">
      <div class="branch-wrap">
        <button class="branch" onclick={() => (branchMenu = !branchMenu)} disabled={!st.remoteUrl}
          title={st.remoteUrl ? t("Branches") : t("Share the project with a team to use branches")}>
          ⑂ {st.branch} ▾
        </button>
        {#if branchMenu}
          <div class="menu" role="menu">
            <div class="menu-h">{t("Switch to")}</div>
            {#each st.branches as b (b.name)}
              <button class="item" disabled={b.current} onclick={() => { branchMenu = false; onswitch(b.name); }}>
                <span>{b.name}</span>
                <span class="faint">{b.current ? t("current") : b.latest ? `${b.latest.author} · ${ago(b.latest.time)}` : ""}</span>
              </button>
            {/each}
            <div class="sep"></div>
            <div class="menu-h">{t("Merge into {branch}", { branch: st.branch })}</div>
            {#each st.branches.filter((b) => !b.current) as b (b.name)}
              <button class="item" onclick={() => { branchMenu = false; onmerge(b.name); }}>{b.name}</button>
            {:else}
              <div class="item faint">{t("no other branches")}</div>
            {/each}
            <div class="sep"></div>
            <button class="item" onclick={() => { branchMenu = false; onnewbranch(); }}>{t("New branch from here…")}</button>
          </div>
        {/if}
      </div>
      {#if st.remoteUrl}
        <span class="dot" class:on={st.online} class:checking={!st.teamChecked}></span>
        <span class="faint" title={st.online ? st.remoteUrl : st.offline}>
          {st.teamName || st.remoteUrl}{!st.teamChecked ? ` · ${t("checking…")}` : st.online ? "" : ` · ${t("not reachable")}`}
        </span>
        {#if preuploads[st.root]}<PreuploadIcon p={preuploads[st.root]} />{/if}
      {/if}
      <button class="ghost gear" class:bad={!!st.rules.error} onclick={onsettings}
        title={st.rules.error ? `${t("Project settings")} — ⚠ ${st.rules.error}` : t("Project settings: name, rules, …")}
        aria-label={t("Project settings")}>{st.rules.error ? "⚠" : ""}<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z"/><circle cx="12" cy="12" r="3"/></svg></button>
    </div>
  </div>
  <div class="actions">
    {#if st.openable.length === 1}
      <button onclick={() => open(st.openable[0])}
        title={t("Open {file} in {tool}", { file: label(st.openable[0]), tool: (st.tool ? t(st.tool) : t("its program")) })}>▶ {t("Open in {tool}", { tool: toolName })}</button>
    {:else if st.openable.length > 1}
      <div class="open-wrap">
        <button onclick={() => (setMenu = !setMenu)} title={t("Open in {tool}", { tool: (st.tool ? t(st.tool) : t("its program")) })}>▶ {t("Open in {tool}", { tool: toolName })} ▾</button>
        {#if setMenu}
          <div class="menu right" role="menu">
            {#each st.openable as s}
              <button class="item" onclick={() => { setMenu = false; open(s); }}>{label(s)}</button>
            {/each}
          </div>
        {/if}
      </div>
    {/if}
    <button class="ghost" onclick={oncheck} title={t("Check the project: Live version, samples, plugins")}>✓</button>
    <button class="ghost refresh" class:spin={refreshing} onclick={onrefresh} title={t("Refresh")}>↻</button>
    <button class="ghost" onclick={() => api.ShowFolder(st.root)} title={t("Show folder")}>📁</button>
  </div>
</header>

<style>
  header { display: flex; align-items: flex-start; padding: 18px 24px 10px; gap: 16px; }
  .title { flex: 1; min-width: 0; }
  h1 { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  h1 { margin: 0 0 6px; font-size: 22px; font-weight: 650; }
  .sub { display: flex; align-items: center; gap: 10px; }
  .actions { display: flex; gap: 8px; flex: none; }
  .refresh.spin { animation: spin .8s linear infinite; }
  @keyframes spin { to { transform: rotate(360deg); } }
  .open-wrap { position: relative; }
  .menu.right { left: auto; right: 0; min-width: 220px; max-height: 50vh; overflow: auto; }
  .branch { padding: 3px 10px; font-size: 13px; }
  .branch-wrap { position: relative; }
  .menu {
    position: absolute; top: 32px; left: 0; z-index: 20; min-width: 260px; padding: 6px;
    background: var(--panel-2); border: 1px solid var(--line); border-radius: 8px;
    box-shadow: 0 12px 30px rgba(0, 0, 0, .45);
  }
  .menu-h { font-size: 11px; text-transform: uppercase; letter-spacing: .06em; color: var(--faint); padding: 6px 8px 2px; }
  .item { display: flex; justify-content: space-between; width: 100%; border: none; background: transparent; padding: 6px 8px; text-align: left; gap: 12px; }
  .item:hover:not(:disabled) { background: #33363d; }
  .sep { height: 1px; background: var(--line); margin: 6px 0; }
  .dot { width: 8px; height: 8px; border-radius: 50%; background: var(--danger); }
  .dot.on { background: var(--accent); }
  .dot.checking { background: var(--faint); }
  .gear { display: inline-flex; align-items: center; gap: 3px; padding: 3px 6px; font-size: 12.5px; color: var(--faint); }
  .gear svg { width: 15px; height: 15px; }
  .gear:hover { color: var(--text); }
  .gear.bad { color: var(--warn); }
</style>
