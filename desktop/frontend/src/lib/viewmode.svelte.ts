// How a selected file is looked at, the same for every kind of file and
// remembered:
//   preview — the file as it is (one state, nothing to compare);
//   changes — what isn't committed yet, against the version you're on;
//   history — the committed versions, each against the one before.
// What a file looks like in each is up to its viewer (see viewers/).
export type FileMode = "preview" | "changes" | "history";

const KEY = "dawgit.fileMode";
const read = () => { try { return localStorage.getItem(KEY) ?? ""; } catch { return ""; } };

export const fileView = $state({
  mode: (["preview", "history"].includes(read()) ? read() : "changes") as FileMode,
});

export function setFileMode(m: FileMode) {
  fileView.mode = m;
  try { localStorage.setItem(KEY, m); } catch { /* not remembered */ }
}
