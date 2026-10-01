// Look at one version of a file (preview) or compare it with the one before,
// for every kind of file: remembered, shared by all the views.
const COMPARE = "dawgit.compareImages", MODE = "dawgit.compareMode";
const read = (k: string) => { try { return localStorage.getItem(k) ?? ""; } catch { return ""; } };
const write = (k: string, v: string) => { try { localStorage.setItem(k, v); } catch { /* not remembered */ } };

export const view = $state({
  compare: read(COMPARE) === "1",
  imageMode: (read(MODE) === "slider" ? "slider" : "side") as "side" | "slider", // how pictures compare
});

export function setCompare(on: boolean) {
  view.compare = on;
  write(COMPARE, on ? "1" : "0");
}

export function setImageMode(m: "side" | "slider") {
  view.imageMode = m;
  write(MODE, m);
}
