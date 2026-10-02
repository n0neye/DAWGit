<script lang="ts">
  import { api, errorText, lineKind } from "./api";
  import type { SetView } from "../../bindings/dawgit/desktop/models";
  import type { ClipSummary, Overview, TrackSummary } from "../../bindings/dawgit/internal/als/models";
  import { setLook, setSetPane } from "./compare.svelte";
  import { inkOn, liveColor } from "./livecolors";

  // A Live Set drawn the way Live shows it, simplified: its tracks with their
  // colors, clips and mixer state, in Arrangement or Session view. Compare
  // shows only the tracks that changed: new, deleted, or changed (with the
  // clips that went away as outlines, the new ones ringed).
  let { root, file, version, fromFile = "", fromVersion, compare, stamp = 0, text = [], textEmpty = "No changes in the set's tracks." }: {
    root: string; file: string;
    version: string;     // "" the project folder now, a version id, "none": no set
    fromFile?: string;   // the set's path in fromVersion, when it was elsewhere
    fromVersion: string; // the version to compare with, "none": nothing
    compare: boolean;
    stamp?: number;      // reload
    text?: string[] | null; // the changes as text (Text view; null: still comparing)
    textEmpty?: string;     // ... when there are none
  } = $props();

  let data = $state<SetView | null>(null);
  let err = $state("");
  let gen = 0;
  let shownKey = "", shownJSON = "";
  // A reload of the same set (the project refreshed) keeps what is shown
  // until the new one is read, and changes nothing when it is the same: no
  // flash. Another set or version starts empty.
  $effect(() => {
    const g = ++gen;
    stamp;
    const key = [root, file, version, fromFile, fromVersion].join("|");
    if (key !== shownKey) {
      shownKey = key;
      shownJSON = "";
      data = null;
      err = "";
    }
    api.SetOverview(root, file, version, fromFile === file ? "" : fromFile, fromVersion)
      .then((d) => {
        if (g !== gen) return;
        const json = JSON.stringify(d);
        if (json === shownJSON) return;
        shownJSON = json;
        data = d;
        err = "";
      })
      .catch((e) => { if (g === gen) err = errorText(e); });
  });

  type Status = "added" | "removed" | "modified";
  type Row = { t: TrackSummary; depth: number; status?: Status; old?: TrackSummary; details: string[]; label: string };

  // groups opened or closed here (else as saved in the set)
  let folds = $state<Record<string, boolean>>({});
  const folded = (t: TrackSummary) => folds[t.id] ?? t.folded;

  let shown = $derived(data ? (data.now ?? data.before) : null);

  function byID(s: Overview | null | undefined) {
    const m = new Map<string, TrackSummary>();
    for (const t of s?.tracks ?? []) m.set(t.id, t);
    return m;
  }

  // The number on a track's activator (1, 2, …; returns A, B, …).
  function labels(s: Overview | null | undefined) {
    const m = new Map<string, string>();
    let n = 0, r = 0;
    for (const t of s?.tracks ?? []) m.set(t.id, t.kind === "return" ? String.fromCharCode(65 + r++) : String(++n));
    return m;
  }

  // Preview: every track, children under their (open) groups.
  let previewRows = $derived.by((): Row[] => {
    if (!shown) return [];
    const ids = byID(shown), lab = labels(shown);
    const depth = (t: TrackSummary): number => (t.group && ids.has(t.group) ? depth(ids.get(t.group)!) + 1 : 0);
    const hidden = (t: TrackSummary): boolean => {
      const g = t.group ? ids.get(t.group) : undefined;
      return !!g && (folded(g) || hidden(g));
    };
    return shown.tracks.filter((t) => !hidden(t)).map((t) => ({ t, depth: depth(t), details: [], label: lab.get(t.id) ?? "" }));
  });

  // Compare: the tracks that changed; deleted ones where they were.
  let compareRows = $derived.by((): Row[] => {
    if (!data) return [];
    const { now, before } = data;
    if (!now && !before) return [];
    if (!before || !now) {
      const s = (now ?? before)!, lab = labels(s);
      return s.tracks.map((t) => ({ t, depth: 0, status: (now ? "added" : "removed") as Status, details: [], label: lab.get(t.id) ?? "" }));
    }
    const change = new Map(data.changes.map((c) => [c.id, c]));
    const old = byID(before), labNow = labels(now), labOld = labels(before);
    const rows: Row[] = [];
    for (const t of now.tracks) {
      const c = change.get(t.id);
      if (c) rows.push({ t, depth: 0, status: c.status as Status, old: old.get(t.id), details: c.status === "added" ? [] : c.details, label: labNow.get(t.id) ?? "" });
    }
    // a deleted track goes after the changed track it followed, if any
    const nowIDs = new Set(now.tracks.map((t) => t.id));
    before.tracks.forEach((t, i) => {
      if (nowIDs.has(t.id)) return;
      const row: Row = { t, depth: 0, status: "removed", details: [], label: labOld.get(t.id) ?? "" };
      let at = 0;
      for (let k = i - 1; k >= 0; k--) {
        const j = rows.findIndex((r) => r.t.id === before.tracks[k].id);
        if (j >= 0) { at = j + 1; break; }
      }
      rows.splice(at, 0, row);
    });
    return rows;
  });

  let rows = $derived(compare ? compareRows : previewRows);
  let sets = $derived(compare ? [data?.now, data?.before] : [shown]);

  // ---- arrangement ----
  let beatsPerBar = $derived(shown ? (shown.timeSig[0] * 4) / shown.timeSig[1] : 4);
  let bars = $derived.by(() => {
    let end = 0;
    for (const s of sets) {
      end = Math.max(end, s?.length ?? 0, ...(s?.locators ?? []).map((l) => l.time));
    }
    return Math.max(4, Math.ceil(end / beatsPerBar));
  });
  let total = $derived(bars * beatsPerBar);
  let step = $derived([1, 2, 4, 8, 16, 32, 64, 128, 256].find((s) => bars / s <= 16) ?? 512);
  const pct = (beats: number) => `${(beats / total) * 100}%`;

  const arrClips = (t: TrackSummary | undefined) => (t?.clips ?? []).filter((c) => c.slot < 0);

  // A group's lane: its tracks' clips, faint.
  function inside(s: Overview | null | undefined, group: string): TrackSummary[] {
    const out: TrackSummary[] = [];
    for (const t of s?.tracks ?? []) {
      if (t.group === group) out.push(t, ...inside(s, t.id));
    }
    return out;
  }
  const setOf = (r: Row) => (r.status === "removed" ? (data?.before ?? shown) : (data?.now ?? shown));

  // ---- session ----
  let scenes = $derived(Math.max(0, ...sets.map((s) => s?.scenes.length ?? 0)));
  let sceneNames = $derived((data?.now ?? shown)?.scenes ?? []);
  function slotClip(t: TrackSummary | undefined, i: number) {
    return t?.clips.find((c) => c.slot === i);
  }
  function groupHasSlot(s: Overview | null | undefined, g: TrackSummary, i: number) {
    return inside(s, g.id).some((t) => t.clips.some((c) => c.slot === i));
  }
  // A group's tracks' clips in one scene.
  function groupClips(s: Overview | null | undefined, g: TrackSummary, i: number) {
    return inside(s, g.id).flatMap((t) => t.clips.filter((c) => c.slot === i));
  }
  // The color that ties a group's columns together: the group's own (for
  // it and the tracks in it), "" for a track in no group.
  function bandOf(r: Row): string {
    if (r.t.kind === "group") return liveColor(r.t.color);
    const g = r.t.group ? byID(setOf(r)).get(r.t.group) : undefined;
    return g ? liveColor(g.color) : "";
  }

  // ---- bits ----
  const db = (v: number) => (v <= -999 ? "-inf" : `${v > 0 ? "+" : ""}${v.toFixed(1)}`);
  const kindName: Record<string, string> = { midi: "MIDI track", audio: "Audio track", group: "Group track", return: "Return track", main: "Main" };
  const statusName: Record<Status, string> = { added: "New", removed: "Deleted", modified: "Changed" };
  const markOf: Record<Status, string> = { added: "+", removed: "−", modified: "~" };
  const clipCount = (t: TrackSummary) => {
    const a = t.clips.filter((c) => c.slot < 0).length, s = t.clips.length - a;
    return [a ? `${a} in arrangement` : "", s ? `${s} in session` : ""].filter(Boolean).join(", ") || "no clips";
  };
  let counts = $derived.by(() => {
    if (!shown) return "";
    const n = shown.tracks.filter((t) => t.kind !== "return").length, r = shown.tracks.length - n;
    return [`${n} track${n === 1 ? "" : "s"}`, r ? `${r} return${r === 1 ? "" : "s"}` : "", `${shown.scenes.length} scene${shown.scenes.length === 1 ? "" : "s"}`]
      .filter(Boolean).join(" · ");
  });
  // Compare: the text of what changed, per track (click its name) or all.
  let detailsOpen = $state<Record<string, boolean>>({});
  let sessionMarks = $derived(rows.map((r) => clipMarks(r, true)));
  let allDetails = $state(false);
  const showDetails = (r: Row) => allDetails || !!detailsOpen[r.t.id];
  const toggleDetails = (r: Row) => { if (compare && r.details.length) detailsOpen[r.t.id] = !detailsOpen[r.t.id]; };

  // How a clip changed, said briefly ([] when it didn't).
  function clipChanges(o: ClipSummary, c: ClipSummary): string[] {
    const out: string[] = [];
    if (o.disabled !== c.disabled) out.push(c.disabled ? "Deactivated" : "Activated");
    if (o.name !== c.name) out.push(`Renamed, was “${o.name}”`);
    if (o.start !== c.start || o.end !== c.end) out.push("Moved or resized");
    if (o.notes !== c.notes) out.push("Notes changed");
    if (o.sample !== c.sample) out.push(`Sample was ${o.sample}`);
    if (o.gain !== c.gain) out.push(`Gain ${db(o.gain)} → ${db(c.gain)} dB`);
    if (o.warp !== c.warp) out.push(c.warp ? "Warp on" : "Warp off");
    else if (o.warpMode !== c.warpMode) out.push(`Warp mode ${o.warpMode} → ${c.warpMode}`);
    if (o.markers !== c.markers) out.push("Warp markers changed");
    if (o.transpose !== c.transpose) out.push(`Transpose ${o.transpose} → ${c.transpose} st`);
    if (o.loop !== c.loop) out.push("Loop changed");
    if (o.fades !== c.fades) out.push("Fades changed");
    if (o.envelopes !== c.envelopes) out.push("Clip automation changed");
    if (o.color !== c.color) out.push("Color changed");
    return out;
  }

  type Mark = { kind: "add" | "mod" | "del"; tip: string };
  // Each clip of a row as compared: new (green), changed (blue, with how),
  // and the clips that went away (red, drawn as outlines).
  function clipMarks(r: Row, session: boolean): { marks: Map<ClipSummary, Mark>; gone: ClipSummary[] } {
    const marks = new Map<ClipSummary, Mark>();
    const gone: ClipSummary[] = [];
    const mine = (t: TrackSummary | undefined) => (t?.clips ?? []).filter((c) => (c.slot >= 0) === session);
    if (!compare || !r.status) return { marks, gone };
    if (r.status !== "modified") {
      for (const c of mine(r.t)) marks.set(c, r.status === "added" ? { kind: "add", tip: "New clip" } : { kind: "del", tip: "Deleted with the track" });
      return { marks, gone };
    }
    const place = (c: ClipSummary) => (session ? `s${c.slot}` : `a${c.start}`);
    const before = new Map<string, ClipSummary[]>();
    for (const c of mine(r.old)) before.set(place(c), [...(before.get(place(c)) ?? []), c]);
    for (const c of mine(r.t)) {
      const o = before.get(place(c))?.shift();
      if (!o) { marks.set(c, { kind: "add", tip: "New clip" }); continue; }
      const ch = clipChanges(o, c);
      if (ch.length) marks.set(c, { kind: "mod", tip: ch.join(" · ") });
    }
    for (const left of before.values()) gone.push(...left);
    return { marks, gone };
  }

  // What a changed track had before, for the dots on what changed.
  type Was = { name?: string; instrument?: string; instrumentSettings?: boolean; muted?: boolean; solo?: boolean; volume?: number };
  function wasOf(r: Row): Was {
    const o = r.old, t = r.t;
    if (r.status !== "modified" || !o) return {};
    const w: Was = {};
    // Live renumbers the names it makes up: only a given name is renamed
    if ((o.named || t.named) && o.name !== t.name) w.name = o.name;
    if (o.instrument !== t.instrument) w.instrument = o.instrument || "none";
    else if (t.instrument && o.instrumentState !== t.instrumentState) w.instrumentSettings = true;
    if (o.muted !== t.muted) w.muted = o.muted;
    if (o.solo !== t.solo) w.solo = o.solo;
    if (o.volume !== t.volume) w.volume = o.volume;
    return w;
  }
</script>

{#snippet icon(kind: string)}
  <svg class="kicon" viewBox="0 0 12 12" aria-hidden="true">
    {#if kind === "midi"}
      <rect x="1" y="2" width="10" height="8" rx="1" fill="none" stroke="currentColor" />
      <path d="M4 2v5M6 2v5M8 2v5" stroke="currentColor" />
    {:else if kind === "audio"}
      <path d="M1 6h1.5M3 3v6M5 1.5v9M7 4v4M9 2.5v7M11 6h-0.5" stroke="currentColor" stroke-linecap="round" fill="none" />
    {:else if kind === "group"}
      <path d="M1 3.5h3.5l1 1H11v5.5H1z" fill="none" stroke="currentColor" />
    {:else if kind === "return"}
      <path d="M9 3.5H4a2.5 2.5 0 0 0 0 5h5M7 6.5l2 2-2 2" fill="none" stroke="currentColor" stroke-linecap="round" />
    {:else}
      <path d="M2 9h8M3 9V5M6 9V2.5M9 9V6" stroke="currentColor" stroke-linecap="round" />
    {/if}
  </svg>
{/snippet}

{#snippet badge(s: Status | undefined)}
  {#if s}<span class="badge {s}">{statusName[s]}</span>{/if}
{/snippet}

{#snippet dot(title: string, kind: string = "mod")}
  <span class="dot {kind}" {title}></span>
{/snippet}

{#snippet clipDot(m: Mark | undefined)}
  {#if m}<span class="dot {m.kind} in" title={m.tip}></span>{/if}
{/snippet}

{#snippet mixer(t: TrackSummary, label: string, w: Was = {})}
  <span class="vol pin" title={w.volume !== undefined ? `Volume, was ${db(w.volume)} dB` : "Volume"}>
    {db(t.volume)}{#if w.volume !== undefined}{@render dot(`Volume was ${db(w.volume)} dB`)}{/if}</span>
  {#if t.kind !== "main"}
    <span class="ctl">
      <span class="act" class:off={t.muted} title={t.muted ? "Track off (muted)" : "Track on"}>{label}</span>
      {#if w.muted !== undefined}{@render dot(w.muted ? "Was off (muted)" : "Was on")}{/if}
    </span>
    <span class="ctl">
      <span class="solo" class:on={t.solo} title={t.solo ? "Soloed" : "Solo"}>S</span>
      {#if w.solo !== undefined}{@render dot(w.solo ? "Was soloed" : "Wasn't soloed")}{/if}
    </span>
  {/if}
{/snippet}

{#snippet trackName(r: Row, w: Was)}
  <span class="nm pin"><span class="tname">{r.t.name}</span>
    {#if w.name !== undefined}{@render dot(`Renamed, was “${w.name}”`)}{/if}</span>
  {#if r.t.instrument || w.instrument !== undefined}
    <span class="instw pin">
      {#if r.t.instrument}<span class="inst" title={r.t.instrumentFull}>{r.t.instrument}</span>{:else}<span class="inst none">no instrument</span>{/if}
      {#if w.instrument !== undefined}{@render dot(`Instrument was ${w.instrument}`)}
      {:else if w.instrumentSettings}{@render dot(`${r.t.instrument}'s settings changed`)}{/if}
    </span>
  {/if}
{/snippet}

<div class="setview">
  <div class="bar">
    <div class="modes" title="Live's two views, or the changes as text">
      <button class:on={setLook.pane === "arrangement"} onclick={() => setSetPane("arrangement")}>Arrangement</button>
      <button class:on={setLook.pane === "session"} onclick={() => setSetPane("session")}>Session</button>
      <button class:on={setLook.pane === "text"} onclick={() => setSetPane("text")}>Text</button>
    </div>
    {#if compare && setLook.pane !== "text" && rows.some((r) => r.details.length)}
      <label class="faint small toggle" title="What changed in each track, as text (or click a track's name)">
        <input type="checkbox" bind:checked={allDetails} /> Details
      </label>
    {/if}
    {#if shown}
      <span class="faint small">{shown.tempo} BPM · {shown.timeSig[0]}/{shown.timeSig[1]} · {counts}</span>
    {/if}
  </div>

  {#if setLook.pane === "text"}
    {#if text === null}
      <p class="muted">Comparing…</p>
    {:else if !text.length}
      <p class="muted">{textEmpty}</p>
    {:else}
      <div class="lines mono">
        {#each text as line}
          <div class={lineKind(line)} style:padding-left="{(line.length - line.trimStart().length) * 4 + 4}px">{line.trim()}</div>
        {/each}
      </div>
    {/if}
  {:else if err}
    <p class="muted">Couldn't read the set: {err}</p>
  {:else if !data}
    <p class="muted">Reading the set…</p>
  {:else if !shown}
    <p class="muted">No set to show.</p>
  {:else}
    {#if compare}
      {#if !data.before && data.now}
        <p class="muted small">A new set: every track is new.</p>
      {:else if data.before && !data.now}
        <p class="muted small">The set was deleted: these were its tracks.</p>
      {:else if data.before && data.now}
        {#if data.global.length || data.order}
          <ul class="global">
            {#each data.global as g}<li><span class="badge modified">Set</span> {g}</li>{/each}
            {#if data.order}<li><span class="badge modified">Set</span> track order changed</li>{/if}
          </ul>
        {/if}
        {#if !rows.length}
          <p class="muted small">{data.global.length || data.order ? "No track changed." : "No changes in the set's tracks."}</p>
        {/if}
      {/if}
    {/if}

    {#if setLook.pane === "arrangement"}
      {#if rows.length || !compare}
        <div class="arr" style:--grid={pct(step * beatsPerBar)}>
          <div class="row ruler">
            <div class="lane">
              {#each Array.from({ length: Math.ceil(bars / step) }, (_, i) => i * step) as b}
                <span class="tick" style:left={pct(b * beatsPerBar)}>{b + 1}</span>
              {/each}
              {#each shown.locators as l}
                <span class="locator" style:left={pct(l.time)} title={l.name}>▸ {l.name}</span>
              {/each}
            </div>
            <div class="head ruler-head"></div>
          </div>
          {#each rows as r (r.t.id + (r.status ?? ""))}
            {@const t = r.t}
            {@const cm = clipMarks(r, false)}
            {@const w = wasOf(r)}
            <div class="row {t.kind} {r.status ?? ''}" class:muted={t.muted} class:firstreturn={!compare && t.kind === "return" && rows[rows.indexOf(r) - 1]?.t.kind !== "return"}>
              <div class="lane">
                {#if t.kind === "group"}
                  {#each inside(setOf(r), t.id) as child}
                    {#each arrClips(child) as c}
                      <div class="sum" style:left={pct(c.start)} style:width={pct(c.end - c.start)} style:background={liveColor(c.color)}></div>
                    {/each}
                  {/each}
                {/if}
                {#each cm.gone as c}
                  <div class="clip ghost" style:left={pct(c.start)} style:width={pct(c.end - c.start)} style:border-color={liveColor(c.color)}
                    title={`Deleted: ${c.name || "clip"}`}>{@render clipDot({ kind: "del", tip: `Deleted: ${c.name || "clip"}` })}</div>
                {/each}
                {#each arrClips(t) as c}
                  {@const m = cm.marks.get(c)}
                  <div class="clip" class:off={c.disabled}
                    style:left={pct(c.start)} style:width={pct(c.end - c.start)}
                    style:background={c.disabled ? "" : liveColor(c.color)} style:color={c.disabled ? "" : inkOn(c.color)}
                    title={`${c.name || "clip"} · bar ${Math.floor(c.start / beatsPerBar) + 1}${c.disabled ? " · deactivated" : ""}${m ? ` · ${m.tip}` : ""}`}>
                    <span>{c.name}</span>{@render clipDot(m)}
                  </div>
                {/each}
              </div>
              <div class="head" style:padding-left="{r.depth * 10}px"
                title={`${kindName[t.kind]} · ${clipCount(t)}${t.devices.length ? ` · ${t.devices.join(", ")}` : ""}${compare && r.details.length ? " · click for what changed" : ""}`}>
                <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
                <div class="hname" class:clickable={compare && r.details.length > 0} onclick={() => toggleDetails(r)}
                  style:background={liveColor(t.color)} style:color={inkOn(t.color)}>
                  {#if t.kind === "group" && !compare}
                    <button class="fold" onclick={() => (folds[t.id] = !folded(t))} title={folded(t) ? "Show its tracks" : "Hide its tracks"}>{folded(t) ? "▸" : "▾"}</button>
                  {/if}
                  {@render icon(t.kind)}
                  {#if r.status}<span class="mark {r.status}" title={statusName[r.status]}>{markOf[r.status]}</span>{/if}
                  {@render trackName(r, w)}
                  <span class="grow"></span>
                  {#if t.clips.length && !compare}<span class="nclips" title={clipCount(t)}>{t.clips.length}</span>{/if}
                </div>
                {@render mixer(t, r.label, w)}
              </div>
            </div>
            {#if r.details.length && showDetails(r)}
              <div class="details">
                {#each r.details as line}<div>{line.trim()}</div>{/each}
              </div>
            {/if}
          {/each}
          {#if !compare}
            <div class="row main">
              <div class="lane"></div>
              <div class="head">
                <div class="hname" style:background={liveColor(shown.main.color)} style:color={inkOn(shown.main.color)}>
                  {@render icon("main")}<span class="tname">Main</span>
                </div>
                {@render mixer(shown.main, "")}
              </div>
            </div>
          {/if}
        </div>
      {/if}
    {:else if rows.length || !compare}
      <div class="sess-wrap">
        <div class="sess" style:grid-template-columns="repeat({rows.length}, 108px) 116px">
          {#each rows as r (r.t.id + (r.status ?? ""))}
            {@const band = bandOf(r)}
            {@const inner = r.t.kind !== "group" && !!band}
            <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
            <div class="ctitle {r.status ?? ''}" class:clickable={compare && r.details.length > 0} onclick={() => toggleDetails(r)}
              class:inner title={`${kindName[r.t.kind]} · ${clipCount(r.t)}${r.t.instrumentFull ? ` · ${r.t.instrumentFull}` : ""}`}>
              <!-- a track in a group: under the band of the group's color, which runs on from the group's title -->
              {#if inner}<i class="band" style:background={band}></i>{/if}
              <div class="tt" style:background={liveColor(r.t.color)} style:color={inkOn(r.t.color)}>
                {@render trackName(r, wasOf(r))}
                {#if r.t.kind === "group" && !compare}
                  <button class="fold gfold" onclick={(e) => { e.stopPropagation(); folds[r.t.id] = !folded(r.t); }}
                    title={folded(r.t) ? "Show its tracks" : "Hide its tracks"}>{folded(r.t) ? "⊕" : "⊖"}</button>
                {/if}
              </div>
            </div>
          {/each}
          <div class="ctitle"><div class="tt" style:background={liveColor(shown.main.color)} style:color={inkOn(shown.main.color)}>Main</div></div>
          {#if compare}
            {#each rows as r}<div class="cbadge">{@render badge(r.status)}</div>{/each}
            <div></div>
          {/if}
          {#each Array.from({ length: scenes }, (_, i) => i) as i}
            {#each rows as r, ri}
              {@const c = slotClip(r.t, i)}
              {@const m = c ? sessionMarks[ri].marks.get(c) : undefined}
              {@const old = sessionMarks[ri].gone.find((g) => g.slot === i)}
              <div class="slot {r.status ?? ''}" class:muted={r.t.muted}>
                {#if c}
                  <div class="sclip" class:off={c.disabled}
                    style:background={c.disabled ? "" : liveColor(c.color)} style:color={c.disabled ? "" : inkOn(c.color)}
                    title={`${c.name || "clip"}${c.disabled ? " · deactivated" : ""}${m ? ` · ${m.tip}` : ""}`}>
                    ▶ {c.name}{@render clipDot(m)}</div>
                {:else if old}
                  <div class="sclip ghost" style:border-color={liveColor(old.color)} title={`Deleted: ${old.name || "clip"}`}>{old.name}{@render clipDot({ kind: "del", tip: `Deleted: ${old.name || "clip"}` })}</div>
                {:else if r.t.kind === "group" && groupHasSlot(setOf(r), r.t, i)}
                  <!-- as Live: the scene's play button for the group, and its tracks' clips in small -->
                  <div class="gslot" title="Its tracks have clips in this scene">
                    <span class="gplay">▶</span>
                    <span class="gmini">{#each groupClips(setOf(r), r.t, i).slice(0, 4) as gc}<i style:background={liveColor(gc.color)}></i>{/each}</span>
                  </div>
                {:else if r.t.kind !== "return"}
                  <span class="stop"></span>
                {/if}
              </div>
            {/each}
            <div class="scene" title={sceneNames[i] || `Scene ${i + 1}`}>▶ {sceneNames[i] || i + 1}</div>
          {/each}
          {#each rows as r}
            <div class="cmix" class:muted={r.t.muted}>{@render mixer(r.t, r.label, wasOf(r))}</div>
          {/each}
          <div class="cmix">{@render mixer(shown.main, "")}</div>
        </div>
      </div>
      {#if compare}
        {#each rows.filter((r) => r.details.length && showDetails(r)) as r}
          <div class="details"><b>{r.t.name}</b>{#each r.details as line}<div>{line.trim()}</div>{/each}</div>
        {/each}
      {/if}
    {/if}
  {/if}
</div>

<style>
  .setview { display: flex; flex-direction: column; gap: 8px; }
  .lines { padding: 8px 10px; background: var(--bg); border: 1px solid var(--line); border-radius: 6px; line-height: 1.6; user-select: text; }
  .lines :global(.add) { color: var(--add); }
  .lines :global(.del) { color: var(--del); }
  .lines :global(.mod) { color: var(--mod); }
  .bar { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; }
  .small { font-size: 12px; }
  .modes { display: flex; }
  .modes button { padding: 3px 10px; font-size: 12px; border-radius: 0; }
  .modes button:first-child { border-radius: 6px 0 0 6px; }
  .modes button:last-child { border-radius: 0 6px 6px 0; margin-left: -1px; }
  .modes button.on { background: var(--accent); color: var(--accent-ink); border-color: var(--accent); }
  .global { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 3px; font-size: 12.5px; }

  /* Live's dark look */
  .arr { background: #1c1c1c; border: 1px solid #000; border-radius: 6px; overflow: hidden; font-size: 12px; color: #d8d8d8; }
  .row { display: flex; align-items: stretch; height: 26px; border-bottom: 1px solid #121212; }
  .row.firstreturn { border-top: 6px solid #121212; }
  .lane { position: relative; flex: 1; min-width: 0; background-color: #2a2a2a;
    background-image: linear-gradient(90deg, #353535 1px, transparent 1px); background-size: var(--grid) 100%; }
  .row.return .lane, .row.main .lane { background-color: #242424; }
  .row.main { border-top: 6px solid #121212; border-bottom: none; }
  .head { flex: 0 0 clamp(220px, 50%, 340px); display: flex; align-items: center; gap: 4px; padding-right: 4px; min-width: 0;
    background: #333; }
  .hname { flex: 1; min-width: 0; align-self: stretch; display: flex; align-items: center; gap: 5px; padding: 0 6px; margin: 2px 2px 2px 0; border-radius: 2px; }
  .row.group .hname { font-weight: 650; }
  .row.muted .hname { filter: saturate(.35) brightness(.75); }
  .hname .nclips { color: inherit; opacity: .7; }
  .hname .kicon { opacity: .85; }
  .ruler { height: 20px; background: #202020; }
  .ruler .lane { background: #202020; }
  .ruler-head { background: #202020; }
  .tick { position: absolute; top: 3px; font-size: 10.5px; color: #8a8a8a; padding-left: 3px; border-left: 1px solid #555; line-height: 14px; }
  .locator { position: absolute; top: 3px; font-size: 10.5px; color: #e9e9e9; white-space: nowrap; transform: translateX(-3px); }
  .clip { position: absolute; top: 2px; bottom: 2px; border-radius: 2px; overflow: hidden; min-width: 2px;
    font-size: 10.5px; line-height: 21px; padding: 0 4px; white-space: nowrap; box-shadow: inset 0 0 0 1px rgba(0, 0, 0, .35); }
  .clip span { opacity: .9; }
  .clip.off, .sclip.off { background: #555; color: #999; }
  /* what changed: a small dot over its top right corner, taking no room
     (green: new, blue: changed, red: deleted) */
  .pin { position: relative; }
  .dot { position: absolute; top: -3px; right: -4px; width: 7px; height: 7px; border-radius: 50%; background: var(--mod);
    box-shadow: 0 0 0 1.5px rgba(0, 0, 0, .6); pointer-events: auto; z-index: 2; }
  .dot.add { background: var(--add); }
  .dot.del { background: var(--del); }
  .dot.in { top: 2px; right: 2px; } /* inside a clip's corner: clips clip */
  .ctl { position: relative; display: inline-flex; align-items: center; flex: none; }
  .nm { display: flex; min-width: 48px; flex: 0 1 auto; }
  .instw { display: flex; min-width: 22px; flex: 0 3 auto; max-width: 45%; }
  .inst.none { opacity: .6; font-style: italic; }
  .inst { flex: 0 1 auto; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    font-size: 10px; font-weight: 600; padding: 0 5px; line-height: 14px; border-radius: 7px; background: rgba(0, 0, 0, .28); color: #f0f0f0; }
  .clickable { cursor: pointer; }
  .toggle { display: inline-flex; align-items: center; gap: 4px; cursor: pointer; }
  .toggle input { margin: 0; }
  .clip.ghost, .sclip.ghost { background: transparent; border: 1.5px dashed; box-shadow: none; opacity: .8; }
  .sum { position: absolute; bottom: 3px; height: 5px; opacity: .55; border-radius: 1px; }
  .row.muted .lane > :not(.ghost) { filter: saturate(.25) brightness(.7); }
  .row.removed { opacity: .6; }
  .row.removed .lane { background-image: repeating-linear-gradient(135deg, transparent 0 6px, rgba(255, 255, 255, .04) 6px 12px); }
  .kicon { width: 12px; height: 12px; flex: none; opacity: .8; }
  .tname { flex: 0 1 auto; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; min-width: 0; }
  .grow { flex: 1; }
  .nclips { font-size: 10.5px; color: #9a9a9a; }
  .vol { font-size: 10.5px; color: #c8c8c8; min-width: 32px; text-align: right; font-variant-numeric: tabular-nums; }
  .act, .solo { flex: none; min-width: 18px; height: 16px; line-height: 16px; text-align: center; font-size: 10px; border-radius: 2px;
    background: #f3c13a; color: #111; font-weight: 600; }
  .act.off { background: #4a4a4a; color: #aaa; }
  .solo { background: #4a4a4a; color: #aaa; }
  .solo.on { background: #4aa3ff; color: #111; }
  .fold { border: none; background: transparent; color: inherit; padding: 0 2px; font-size: 10px; cursor: pointer; }
  .mark { flex: none; width: 14px; height: 14px; line-height: 14px; text-align: center; border-radius: 3px; font-size: 11px; font-weight: 700; }
  .mark.added { background: var(--add); color: #111; }
  .mark.removed { background: var(--del); color: #111; }
  .mark.modified { background: var(--mod); color: #111; }
  .badge { flex: none; font-size: 10px; font-weight: 600; padding: 1px 6px; border-radius: 8px; line-height: 14px; }
  .badge.added { background: color-mix(in srgb, var(--add) 25%, transparent); color: var(--add); }
  .badge.removed { background: color-mix(in srgb, var(--del) 25%, transparent); color: var(--del); }
  .badge.modified { background: color-mix(in srgb, var(--mod) 25%, transparent); color: var(--mod); }
  .details { font-size: 11.5px; color: var(--muted); padding: 4px 10px 6px 12px; background: var(--bg); border-bottom: 1px solid var(--line);
    font-family: ui-monospace, Consolas, monospace; line-height: 1.5; }
  .arr .details { background: #1a1a1a; border-bottom-color: #121212; color: #a8a8a8; }
  .details b { font-family: inherit; color: var(--text); }
  .link { border: none; background: transparent; color: var(--accent); padding: 0; font-size: 11.5px; cursor: pointer; }

  .sess-wrap { overflow-x: auto; background: #1c1c1c; border: 1px solid #000; border-radius: 6px; }
  .sess { display: grid; gap: 1px; background: #121212; font-size: 11px; color: #d8d8d8; width: max-content; }
  .ctitle { display: flex; flex-direction: column; height: 26px; font-weight: 600; min-width: 0; }
  .ctitle .tt { flex: 1; display: flex; align-items: center; gap: 3px; padding: 0 6px; overflow: hidden; min-width: 0; }
  .ctitle.removed { opacity: .6; }
  .cbadge { background: #1c1c1c; padding: 2px 4px; text-align: center; }
  .slot { height: 20px; background: #2a2a2a; display: flex; align-items: center; padding: 0 2px; min-width: 0; }
  .slot.removed { opacity: .6; }
  .slot.muted .sclip:not(.ghost) { filter: saturate(.25) brightness(.7); }
  .sclip { flex: 1; min-width: 0; height: 16px; line-height: 16px; padding: 0 4px; border-radius: 2px; overflow: hidden;
    white-space: nowrap; text-overflow: ellipsis; font-size: 10.5px; box-shadow: inset 0 0 0 1px rgba(0, 0, 0, .35); }
  /* a group's slot: play button and its tracks' clips, small and hatched */
  .gslot { flex: 1; display: flex; align-items: center; justify-content: space-between; padding: 0 3px 0 4px; min-width: 0; }
  .gplay { font-size: 9px; color: #9a9a9a; }
  .gmini { display: flex; gap: 2px; }
  .gmini i { width: 9px; height: 12px; border-radius: 1px;
    background-image: repeating-linear-gradient(135deg, rgba(0, 0, 0, .38) 0 1.5px, transparent 1.5px 3.5px); }
  /* columns of a group: a band of its color over the titles, as in Live */
  /* the band reaches over the 1px gap to its left, so it runs on unbroken from the group's title */
  .ctitle .band { display: block; flex: none; height: 5px; margin: 0 0 2px -1px; }
  .gfold { margin-left: auto; font-size: 12px; line-height: 1; padding: 0 2px; opacity: .85; }
  .stop { width: 7px; height: 7px; background: #4a4a4a; margin-left: 4px; border-radius: 1px; }
  .scene { height: 20px; line-height: 20px; padding: 0 6px; background: #333; overflow: hidden; white-space: nowrap; text-overflow: ellipsis; }
  .cmix { display: flex; align-items: center; justify-content: flex-end; gap: 4px; padding: 4px; background: #333; }
  .cmix.muted { background: #2b2b2b; }
</style>
