// Each viewer's own settings, remembered on this computer. They change how
// a kind of file is shown, never what is looked at.
const read = (k: string) => { try { return localStorage.getItem(k) ?? ""; } catch { return ""; } };
const write = (k: string, v: string) => { try { localStorage.setItem(k, v); } catch { /* not remembered */ } };

// Pictures compared side by side or under a slider.
export const imageLook = $state({ mode: (read("dawgit.compareMode") === "slider" ? "slider" : "side") as "side" | "slider" });
export function setImageMode(m: "side" | "slider") {
  imageLook.mode = m;
  write("dawgit.compareMode", m);
}

// Live Sets: Live's Arrangement or Session view, or text; and, when
// comparing, every track or only the ones that changed.
export type SetPane = "arrangement" | "session" | "text";
export const setLook = $state({
  pane: (["session", "text"].includes(read("dawgit.setPane")) ? read("dawgit.setPane") : "arrangement") as SetPane,
  allTracks: read("dawgit.setAllTracks") === "1",
});
export function setSetPane(p: SetPane) {
  setLook.pane = p;
  write("dawgit.setPane", p);
}
export function setAllTracks(on: boolean) {
  setLook.allTracks = on;
  write("dawgit.setAllTracks", on ? "1" : "0");
}
