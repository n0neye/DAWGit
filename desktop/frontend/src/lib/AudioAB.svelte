<script lang="ts">
  import { t } from "./i18n.svelte";
  import Waveform from "./Waveform.svelte";

  // Two versions of a sample, one above the other, on one time scale: a play
  // button, the waveform and the time in one strip. Starting one pauses the
  // other and picks up at the same position, to hear the difference. Space
  // plays or pauses the one you used last.
  let { a, b }: {
    a: { label: string; src: string } | null; // newer
    b: { label: string; src: string } | null; // older
  } = $props();

  type Side = "a" | "b";
  let els = $state<Record<Side, HTMLAudioElement | undefined>>({ a: undefined, b: undefined });
  let playing = $state<Record<Side, boolean>>({ a: false, b: false });
  let now = $state<Record<Side, number>>({ a: 0, b: 0 });
  let durations = $state<Record<Side, number>>({ a: 0, b: 0 });
  let failed = $state<Record<Side, boolean>>({ a: false, b: false });
  let active = $state<Side>("a");
  let longest = $derived(Math.max(durations.a, durations.b));

  const other = (s: Side): Side => (s === "a" ? "b" : "a");

  function toggle(s: Side) {
    const el = els[s];
    if (!el) return;
    active = s;
    if (el.paused) el.play();
    else el.pause();
  }

  function onplay(s: Side) {
    active = s;
    playing[s] = true;
    const o = els[other(s)];
    if (o && !o.paused) { // A/B: continue where the other one was
      els[s]!.currentTime = Math.min(o.currentTime, els[s]!.duration || o.currentTime);
      o.pause();
    }
  }

  function key(e: KeyboardEvent) {
    const t = e.target as HTMLElement;
    if (e.code !== "Space" || t.closest("input, textarea, select, [contenteditable]")) return;
    const s = els[active] ? active : els.a ? "a" : "b";
    if (!els[s]) return;
    e.preventDefault(); // no page scroll, no button click
    toggle(s);
  }

  function fmt(s: number): string {
    const m = Math.floor(s / 60), sec = s - m * 60;
    return `${m}:${sec.toFixed(1).padStart(4, "0")}`;
  }

  $effect(() => () => { els.a?.pause(); els.b?.pause(); }); // stop when the view goes
</script>

<svelte:window onkeydown={key} />

<div class="ab">
  {#each [["a", a], ["b", b]] as [s, side] (s)}
    {@const k = s as Side}
    {#if side && typeof side === "object"}
      <div class="take" class:active={active === k && (els.a && els.b ? true : false)}>
        <div class="label">
          <span>{side.label}</span>
          {#if durations[k]}<span class="faint">{fmt(durations[k])}</span>{/if}
        </div>
        {#if failed[k]}
          <p class="faint small">{t("This file can't be played here.")}</p>
        {:else}
          <div class="strip" role="group" onpointerdown={() => (active = k)}>
            <button class="play" class:on={playing[k]} onclick={() => toggle(k)}
              title={playing[k] ? "Pause (Space)" : "Play (Space)"} aria-label={playing[k] ? "Pause" : "Play"}>
              {#if playing[k]}
                <svg viewBox="0 0 16 16"><rect x="3.5" y="2.5" width="3" height="11" rx="1" /><rect x="9.5" y="2.5" width="3" height="11" rx="1" /></svg>
              {:else}
                <svg viewBox="0 0 16 16"><path d="M4.5 2.8 L13 8 L4.5 13.2 Z" /></svg>
              {/if}
            </button>
            <div class="lane">
              <div class="fill" style:width="{longest && durations[k] ? (durations[k] / longest) * 100 : 100}%">
                <Waveform src={side.src} audio={els[k]} onduration={(d) => (durations[k] = d)} />
              </div>
            </div>
            <div class="time">{fmt(now[k])}</div>
            <audio bind:this={els[k]} preload="metadata" src={side.src}
              onplay={() => onplay(k)} onpause={() => (playing[k] = false)} onended={() => (playing[k] = false)}
              ontimeupdate={() => (now[k] = els[k]?.currentTime ?? 0)} onerror={() => (failed[k] = true)}></audio>
          </div>
        {/if}
      </div>
    {/if}
  {/each}
  {#if a && b}
    <p class="faint small">{t("Space plays or pauses · start the other one to switch at the same position · click the waveform to jump")}</p>
  {:else if a || b}
    <p class="faint small">{t("Space plays or pauses · click the waveform to jump")}</p>
  {/if}
</div>

<style>
  .ab { display: flex; flex-direction: column; gap: 12px; }
  .take { display: flex; flex-direction: column; gap: 6px; }
  .label { display: flex; justify-content: space-between; gap: 8px; font-size: 12px; text-transform: uppercase;
    letter-spacing: .06em; color: var(--muted); }
  .label .faint { text-transform: none; letter-spacing: 0; font-variant-numeric: tabular-nums; }
  .strip {
    display: flex; align-items: center; gap: 12px; height: 72px; padding: 0 12px 0 10px;
    background: var(--bg); border: 1px solid var(--line); border-radius: 10px; transition: border-color .15s;
  }
  .take.active .strip { border-color: #3a5a52; }
  .play {
    flex: none; width: 40px; height: 40px; border-radius: 50%; padding: 0; display: grid; place-items: center;
    background: var(--panel-2); border: 1px solid var(--line); color: var(--text);
  }
  .play:hover { border-color: var(--accent); color: var(--accent); }
  .play.on { background: var(--accent); border-color: var(--accent); color: var(--accent-ink); }
  .play svg { width: 16px; height: 16px; fill: currentColor; }
  .lane { flex: 1; min-width: 0; height: 56px; }
  .fill { height: 100%; }
  .time { flex: none; width: 52px; text-align: right; font-size: 12.5px; color: var(--muted); font-variant-numeric: tabular-nums; }
  .small { font-size: 12px; margin: 0; }
</style>
