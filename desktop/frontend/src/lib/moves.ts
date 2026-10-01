// Moved folders, for the change trees: a folder whose changes are all files
// moved together from one other folder is said to have moved, once, on the
// folder that actually moved.
type Moved = { path: string; status: string; from?: string };

export function folderMoves(list: Moved[]) {
  const changed = list.filter((f) => f.status !== "unchanged" && f.status !== "ignored");
  // Where all of dir's changes came from, if from one folder ("" if not).
  function folderFrom(dir: string): string {
    let from = "";
    let n = 0;
    for (const f of changed) {
      if (!f.path.startsWith(dir + "/")) continue;
      const rest = f.path.slice(dir.length + 1);
      if (f.status !== "renamed" || !f.from || !f.from.endsWith("/" + rest)) return "";
      const old = f.from.slice(0, f.from.length - rest.length - 1);
      if (from && old !== from) return "";
      from = old;
      n++;
    }
    // One file moved into another folder is a file move, not a folder's.
    if (n < 2 && base(from) !== base(dir)) return "";
    return from;
  }
  // Every change in dir is inside one of its subfolders, which moved: that
  // subfolder is the one that moved (its parent only holds it).
  function inOneMovedChild(dir: string): boolean {
    let child = "";
    for (const f of changed) {
      if (!f.path.startsWith(dir + "/")) continue;
      const rest = f.path.slice(dir.length + 1);
      const cut = rest.indexOf("/");
      if (cut < 0) return false;
      const c = dir + "/" + rest.slice(0, cut);
      if (child && c !== child) return false;
      child = c;
    }
    return !!child && !!folderFrom(child);
  }
  const memo = new Map<string, string>();
  // Where dir moved from, said on the folder that moved only ("" otherwise).
  function movedFrom(dir: string): string {
    if (memo.has(dir)) return memo.get(dir)!;
    let out = folderFrom(dir);
    if (out && inOneMovedChild(dir)) out = "";
    const cut = dir.lastIndexOf("/");
    if (out && cut >= 0) {
      const up = movedFrom(dir.slice(0, cut));
      if (up && up + dir.slice(cut) === out) out = ""; // moved with its parent
    }
    memo.set(dir, out);
    return out;
  }
  // A file whose move is said by a folder above it.
  function covered(path: string): boolean {
    for (let cut = path.lastIndexOf("/"); cut > 0; cut = path.lastIndexOf("/", cut - 1)) {
      if (movedFrom(path.slice(0, cut))) return true;
    }
    return false;
  }
  return { movedFrom, covered };
}

const base = (p: string) => p.slice(p.lastIndexOf("/") + 1);
