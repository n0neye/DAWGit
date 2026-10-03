<script lang="ts">
  import { t } from "./i18n.svelte";
  import { api, ago, errorText, type Change, type Version } from "./api";
  import { layout } from "./graph";
  import ChangeList from "./ChangeList.svelte";

  // latest: the branch's newest version (differs from head on an older one).
  // onmerge is offered on versions the current branch does not contain yet
  // (other branches); the team's new versions of this branch come with Get updates.
  // Clicking a version shows what it changed.
  let { root, versions, head, incoming, latest = head, ongoto, onexport, onmerge }: {
    root: string; versions: Version[]; head: string; incoming: Set<string>; latest?: string;
    ongoto?: (v: Version) => void; onexport?: (v: Version) => void; onmerge?: (v: Version) => void;
  } = $props();

  const ROW = 40, LANE = 16, PAD = 12;
  // Main lines start at the branch heads and where you are; lines merged in
  // from the side are drawn dashed.
  let rows = $derived(layout(versions, [head, latest, ...versions.filter((v) => v.branches.length).map((v) => v.id)].filter(Boolean)));
  // Merges DAWGit made (the team's versions and yours combined): quiet.
  const autoMerge = (v: { parents: string[]; message: string }) =>
    v.parents.length > 1 && v.message === "Merge versions from the team";
  let graphWidth = $derived(PAD * 2 + LANE * Math.max(1, ...rows.map((r) => r.width)));
  const x = (lane: number) => PAD + lane * LANE;

  // Open versions, what they changed (or why that can't be shown), and how
  // tall their details are, so the graph's lines run past them.
  let open = $state<Record<string, boolean>>({});
  let changes = $state<Record<string, Change[] | string>>({});
  let extra = $state<Record<string, number>>({});

  async function toggle(v: Version) {
    open[v.id] = !open[v.id];
    if (open[v.id] && changes[v.id] === undefined) {
      try {
        changes[v.id] = (await api.VersionChanges(root, v.id)) ?? [];
      } catch (e) {
        changes[v.id] = errorText(e);
      }
    }
  }

  // Top of each row (and the total height at the end).
  let tops = $derived.by(() => {
    const t: number[] = [];
    let y = 0;
    for (const v of versions) {
      t.push(y);
      y += ROW + (open[v.id] ? extra[v.id] ?? 0 : 0);
    }
    t.push(y);
    return t;
  });
  const mid = (i: number) => tops[i] + ROW / 2;
  // From version i down to i+1: straight past i's details, then the usual curve.
  function segment(i: number, from: number, to: number) {
    const y = tops[i + 1] - ROW / 2;
    return `M ${x(from)} ${mid(i)} L ${x(from)} ${y} C ${x(from)} ${y + ROW / 2}, ${x(to)} ${y + ROW / 2}, ${x(to)} ${y + ROW}`;
  }
</script>

{#if versions.length === 0}
  <p class="muted">{t("No versions yet. Commit your first version from the Changes tab.")}</p>
{:else}
  <div class="history">
    <svg width={graphWidth} height={tops[versions.length]} class="graph">
      <defs>
        <filter id="head-glow" x="-150%" y="-150%" width="400%" height="400%">
          <feGaussianBlur stdDeviation="3.5" />
        </filter>
      </defs>
      {#each rows as r, i}
        {#each r.down as s}
          <path d={segment(i, s.from, s.to)} stroke="var(--lane-{s.color})" stroke-width={s.side ? 1.5 : 2} fill="none"
            stroke-dasharray={s.side ? "4 4" : undefined} opacity={s.side ? 0.55 : 1} />
        {/each}
      {/each}
      {#each rows as r, i}
        {#if versions[i].id === head}
          <circle cx={x(r.lane)} cy={mid(i)} r="10" fill="var(--lane-{r.color})" opacity="0.75" filter="url(#head-glow)" />
          <circle class="ring" cx={x(r.lane)} cy={mid(i)} r="9.5" fill="none" stroke="var(--lane-{r.color})" stroke-width="1.5" />
        {/if}
        <circle cx={x(r.lane)} cy={mid(i)} r={versions[i].id === head ? 6 : 4.5}
          fill={incoming.has(versions[i].id) ? "var(--bg)" : `var(--lane-${r.color})`}
          stroke="var(--lane-{r.color})" stroke-width="2" />
      {/each}
    </svg>
    <ul style:padding-left="{graphWidth}px">
      {#each versions as v (v.id)}
        <li class:incoming={incoming.has(v.id)} class:open={open[v.id]} class:automerge={autoMerge(v)}>
          <div class="row" style:height="{ROW}px" role="button" tabindex="0" title={t("Show what this version changed")}
            onclick={() => toggle(v)} onkeydown={(e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); toggle(v); } }}>
            <span class="msg">
              {autoMerge(v) ? t("Combined with the team's versions") : v.message || t("(no description)")}
              {#each v.branches as b}<span class="tag">{b}</span>{/each}
              {#if v.id === head}<span class="tag here">{t("you are here")}</span>{/if}
              {#if v.id === latest && latest !== head}<span class="tag">{t("latest")}</span>{/if}
              {#if incoming.has(v.id)}<span class="tag new">{t("new")}</span>{/if}
              {#if v.notHere}<span class="tag away" title={t("Some of its files are only in the storage of the team this project was in")}>{t("files in team storage")}</span>{/if}
            </span>
            <span class="who">{v.author}</span>
            <span class="when faint">{ago(v.time)}</span>
            <span class="id mono faint">{v.short}</span>
            {#if ongoto || onexport || onmerge}
              <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
              <span class="acts" onclick={(e) => e.stopPropagation()}>
                {#if onmerge && !v.inBranch && !incoming.has(v.id)}
                  <button onclick={() => onmerge(v)} title={t("Merge this version into the branch you are on")}>{t("Merge")}</button>
                {/if}
                {#if ongoto && v.id !== head && !incoming.has(v.id) && !v.notHere}
                  <button onclick={() => ongoto(v)} title={t("Put the project in the state of this version")}>{t("Go to")}</button>
                {/if}
                {#if onexport && !v.notHere}
                  <button onclick={() => onexport(v)} title={t("Save this version as a separate project folder")}>{t("Export…")}</button>
                {/if}
              </span>
            {/if}
          </div>
          {#if open[v.id]}
            <div class="details" bind:clientHeight={extra[v.id]}>
              {#if v.message}<p class="full">{v.message}</p>{/if}
              <div class="meta faint">
                {v.author} · {new Date(v.time).toLocaleString()} · <span class="mono">{v.short}</span>
                {#if v.parents.length > 1} · {t("merge")}{/if}
              </div>
              {#if changes[v.id] === undefined}
                <p class="faint">{t("Reading…")}</p>
              {:else if typeof changes[v.id] === "string"}
                <p class="error">{changes[v.id]}</p>
              {:else}
                <ChangeList changes={changes[v.id] as Change[]} empty={t("No file changes.")} />
              {/if}
            </div>
          {/if}
        </li>
      {/each}
    </ul>
  </div>
{/if}

<style>
  .history { position: relative; }
  .graph { position: absolute; left: 0; top: 0; overflow: visible; }
  ul { list-style: none; margin: 0; }
  li { border-bottom: 1px solid #25272c; }
  .row { position: relative; display: flex; align-items: center; gap: 12px; cursor: pointer; }
  .row:hover, li.open .row { background: rgba(255, 255, 255, .025); }
  .acts {
    position: absolute; right: 0; top: 50%; transform: translateY(-50%); display: none; gap: 6px;
    padding-left: 24px; background: linear-gradient(to right, transparent, var(--bg) 20px);
  }
  .row:hover .acts { display: flex; }
  .acts button { padding: 3px 10px; font-size: 12.5px; }
  li.incoming .msg { color: var(--muted); }
  li.automerge .msg { color: var(--faint); font-style: italic; }
  .msg { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .who { width: 90px; color: var(--muted); }
  .when { width: 110px; text-align: right; }
  .id { width: 80px; text-align: right; }
  .tag {
    display: inline-block; margin-left: 8px; padding: 0 7px; border-radius: 10px; font-size: 11.5px;
    background: #2b3a45; color: #9fd0f5; vertical-align: 1px;
  }
  .tag.here { background: #1f3b35; color: var(--accent); }
  .tag.new { background: var(--warn-bg); color: var(--warn); }
  .tag.away { background: #2a2c31; color: var(--faint); }
  /* Details: a pixel-exact height (the graph follows it), so no collapsing margins. */
  .details { padding: 4px 0 14px; font-size: 13px; display: flow-root; }
  .full { white-space: pre-wrap; margin-bottom: 4px !important; user-select: text; }
  .meta { font-size: 12px; margin-bottom: 10px; }
  .details p { margin: 0; }
  .error { color: var(--danger); }
  .ring { animation: pulse 2.4s ease-in-out infinite; transform-box: fill-box; transform-origin: center; }
  @keyframes pulse { 0%, 100% { opacity: .9; } 50% { opacity: .25; } }
</style>
