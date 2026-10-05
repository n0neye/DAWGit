import { tn } from "./i18n.svelte";
import type { Version } from "./api";

// "Yi took back “x”": versions a teammate took back.
export const takenBackText = (vs: Version[]) => tn(vs.length, "{who} took back “{version}”.", "{who} took back {n} versions.",
  { who: [...new Set(vs.map((v) => v.author))].join(", "), version: vs[0]?.message || vs[0]?.short || "" });

// The team's new versions to tell about: a merge only combines the others.
export const newsOf = (incoming: Version[]) => {
  const own = incoming.filter((v) => v.parents.length < 2);
  return own.length ? own : incoming;
};
