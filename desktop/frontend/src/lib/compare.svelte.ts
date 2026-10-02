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

// Live Sets: drawn as Live shows them, or the changes as text; and which of
// Live's views.
const SETLOOK = "dawgit.setLook", SETPANE = "dawgit.setPane";
export const setLook = $state({
  text: read(SETLOOK) === "text",
  pane: (read(SETPANE) === "session" ? "session" : "arrangement") as "arrangement" | "session",
});
export function setSetText(on: boolean) {
  setLook.text = on;
  write(SETLOOK, on ? "text" : "tracks");
}
export function setSetPane(p: "arrangement" | "session") {
  setLook.pane = p;
  write(SETPANE, p);
}

export function setImageMode(m: "side" | "slider") {
  view.imageMode = m;
  write(MODE, m);
}
