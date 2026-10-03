import { t } from "./i18n.svelte";

// How much a change of a set matters (diff.Weight* in Go), lightest first:
// a plugin re-saving its own state, tidying (names, colors, order, groups),
// mixing, the sound (devices), the arrangement (clips, notes, tracks).
export type Weight = "noise" | "tidy" | "mix" | "sound" | "arrangement";

export const weightOrder: Weight[] = ["arrangement", "sound", "mix", "tidy", "noise"];
const rank: Record<string, number> = { noise: 0, tidy: 1, mix: 2, sound: 3, arrangement: 4 };

export const heavier = (a: string, b: string) => ((rank[b] ?? -1) > (rank[a] ?? -1) ? b : a);
// Small: shown quietly (likely not what someone meant to change).
export const isSmall = (w: string) => w === "noise" || w === "tidy";

export function weightName(w: string): string {
  return ({ arrangement: t("Arranging"), sound: t("Sound"), mix: t("Mix"), tidy: t("Tidying"),
    noise: t("Plugin state") } as Record<string, string>)[w] ?? "";
}

export function weightHint(w: string): string {
  return ({
    arrangement: t("Clips, notes, tracks added or removed, tempo or scenes"),
    sound: t("Instruments and effects added, removed or set differently"),
    mix: t("Volume, pan, sends, routing or automation"),
    tidy: t("Names, colors, order or groups"),
    noise: t("A plugin saved its own state again: often nobody changed anything"),
  } as Record<string, string>)[w] ?? "";
}

export type TrackWeight = { name: string; status: string; weight: string };

// Tracks by weight, heaviest first: [weight, track names].
export function byWeight(tracks: TrackWeight[]): [Weight, string[]][] {
  const m = new Map<Weight, string[]>();
  for (const tr of tracks) {
    const w = (tr.weight || "tidy") as Weight;
    if (!m.has(w)) m.set(w, []);
    if (!m.get(w)!.includes(tr.name)) m.get(w)!.push(tr.name);
  }
  return weightOrder.filter((w) => m.has(w)).map((w) => [w, m.get(w)!]);
}

// "Drums, Bass, Keys +2"
export function names(list: string[], max = 3): string {
  return list.length <= max ? list.join(", ") : `${list.slice(0, max).join(", ")} +${list.length - max}`;
}
