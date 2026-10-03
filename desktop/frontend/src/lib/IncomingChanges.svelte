<script lang="ts">
  import { untrack } from "svelte";
  import { t, tn } from "./i18n.svelte";
  import { api, ago, errorText, type Change, type Preview, type Version } from "./api";
  import SetView from "./SetView.svelte";
  import ChangeList from "./ChangeList.svelte";
  import WeightSummary from "./WeightSummary.svelte";

  // What teammates' versions bring in: each version with what it changed
  // (pick one to see only that), and the sets drawn as Live shows them,
  // compared, the other files in a list. "All" compares where the two sides
  // parted (preview.base) with what comes in (preview.target).
  let { root, preview, versions }: { root: string; preview: Preview; versions: Version[] } = $props();

  const isSet = (p: string) => /\.als$/i.test(p);
  let picked = $state(""); // a version id, "" for all
  // each version's own changes: null while read, a string for an error
  let own = $state<Record<string, Change[] | string | null>>({});

  $effect(() => {
    const want = versions.slice(0, 12);
    untrack(() => {
      for (const v of want) {
        if (own[v.id] !== undefined) continue;
        own[v.id] = null;
        api.VersionChanges(root, v.id)
          .then((cs) => (own[v.id] = cs ?? []))
          .catch((e) => (own[v.id] = errorText(e)));
      }
    });
  });

  let version = $derived(versions.find((v) => v.id === picked));
  let changes = $derived.by((): Change[] => {
    if (!version) return preview.changes;
    const c = own[version.id];
    return Array.isArray(c) ? c : [];
  });
  let from = $derived(version ? (version.parents[0] ?? "none") : preview.base || "none");
  let to = $derived(version ? version.id : preview.target);
  let sets = $derived(changes.filter((c) => isSet(c.path)));
  let others = $derived(changes.filter((c) => !isSet(c.path)));
  let opened = $state<Record<string, boolean>>({});
  const isOpen = (c: Change, i: number) => opened[c.path] ?? i === 0;

  const tracksOf = (cs: Change[] | string | null | undefined) => (Array.isArray(cs) ? cs.flatMap((c) => c.tracks ?? []) : []);
  const otherCount = (cs: Change[] | string | null | undefined) => (Array.isArray(cs) ? cs.filter((c) => !isSet(c.path)).length : 0);
</script>

<ul class="versions">
  {#if versions.length > 1}
    <li><button class="v" class:on={!picked} onclick={() => (picked = "")}>
      <span class="msg">{t("All of it together")}</span>
      <span class="faint">{tn(versions.length, "{n} version", "{n} versions")}</span>
    </button></li>
  {/if}
  {#each versions as v (v.id)}
    <li><button class="v" class:on={picked === v.id || versions.length === 1} onclick={() => (picked = versions.length > 1 ? v.id : "")}>
      <span class="line">
        <span class="msg">{v.message || t("(no description)")}</span>
        <span class="faint">{v.author} · {ago(v.time)}</span>
      </span>
      <span class="what">
        {#if own[v.id] === null}<span class="faint small">…</span>
        {:else if typeof own[v.id] === "string"}<span class="faint">{own[v.id]}</span>
        {:else}
          <WeightSummary tracks={tracksOf(own[v.id])} />
          {#if otherCount(own[v.id])}<span class="faint small">{tn(otherCount(own[v.id]), "{n} other file", "{n} other files")}</span>{/if}
        {/if}
      </span>
    </button></li>
  {/each}
</ul>

{#if version && typeof own[version.id] === "string"}
  <p class="muted">{own[version.id]}</p>
{:else}
  {#each sets as c, i (c.path + to)}
    <div class="set">
      <button class="sethead" onclick={() => (opened[c.path] = !isOpen(c, i))}>
        <span class="chev" class:open={isOpen(c, i)}>▸</span>
        <span class="name">{c.path}</span>
        <span class="faint small">{c.status === "added" ? t("New") : c.status === "deleted" ? t("Deleted") : ""}</span>
      </button>
      {#if isOpen(c, i)}
        <SetView {root} file={c.path} version={c.status === "deleted" ? "none" : to}
          fromFile={c.status === "renamed" ? c.from : ""} fromVersion={c.status === "added" ? "none" : from} compare={true} />
      {/if}
    </div>
  {/each}
  {#if others.length}
    <details class="others" open={!sets.length}>
      <summary>{sets.length ? tn(others.length, "{n} other file", "{n} other files") : tn(others.length, "{n} file", "{n} files")}</summary>
      <ChangeList changes={others} />
    </details>
  {/if}
  {#if !sets.length && !others.length}<p class="muted">{t("No file changes.")}</p>{/if}
{/if}

<style>
  .versions { list-style: none; margin: 0 0 12px; padding: 0; display: flex; flex-direction: column; gap: 2px; }
  .v { width: 100%; text-align: left; display: flex; flex-direction: column; gap: 2px; padding: 6px 10px; border-radius: 6px;
    border: 1px solid transparent; background: transparent; }
  .v:hover { background: var(--panel); }
  .v.on { background: var(--panel-2); border-color: var(--line); }
  .line { display: flex; gap: 10px; width: 100%; }
  .msg { flex: 1; }
  .what { display: flex; flex-wrap: wrap; gap: 4px 12px; align-items: center; }
  .small { font-size: 12px; }
  .set { margin: 0 0 10px; }
  .sethead { display: flex; align-items: center; gap: 6px; width: 100%; padding: 4px 2px; border: none; background: transparent;
    text-align: left; font-size: 13px; }
  .chev { display: inline-block; color: var(--muted); transition: transform .12s; }
  .chev.open { transform: rotate(90deg); }
  .name { font-weight: 600; }
  .others summary { cursor: pointer; color: var(--muted); font-size: 13px; margin: 4px 0; }
</style>
