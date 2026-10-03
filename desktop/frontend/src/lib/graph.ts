// Lane layout for the version history (like `git log --graph`). Versions come
// newest first, each listed before its parents.

// side: a line off the main lines (see layout's tips), drawn dashed.
export type Segment = { from: number; to: number; color: number; side?: boolean };
export type GraphRow = {
  lane: number;    // column of this version's dot
  color: number;
  down: Segment[]; // curves from this row's centre to the next row's centre
  width: number;   // lanes in use
};

// tips: versions the main lines start from (branch heads, where you are):
// a main line follows first parents from a tip; every other line (versions
// merged in from the side) is a side line. Without tips, no line is.
export function layout(versions: { id: string; parents: string[] }[], tips: string[] = []): GraphRow[] {
  const known = new Set(versions.map((v) => v.id));
  const byID = new Map(versions.map((v) => [v.id, v]));
  const main = new Set<string>();
  for (let id of tips) {
    while (id && byID.has(id) && !main.has(id)) {
      main.add(id);
      id = byID.get(id)!.parents[0];
    }
  }
  // Each column's line: side or not (decided where it starts).
  let sides: boolean[] = [];
  const colors = new Map<string, number>();
  let nextColor = 0;
  const colorOf = (id: string) => {
    if (!colors.has(id)) colors.set(id, nextColor++ % 5);
    return colors.get(id)!;
  };
  // lanes[i] = id of the version the line in column i is heading to.
  let lanes: (string | null)[] = [];
  const rows: GraphRow[] = [];

  for (const v of versions) {
    let lane = lanes.indexOf(v.id);
    if (lane < 0) {
      lane = lanes.indexOf(null);
      if (lane < 0) lane = lanes.length;
      lanes[lane] = v.id;
    }
    const color = colorOf(v.id);
    // Other columns heading to this version end here.
    const before = lanes.map((id, i) => (id === v.id && i !== lane ? null : id));

    const after = before.slice();
    after[lane] = null;
    const afterSides = sides.slice();
    const down: Segment[] = [];
    // Columns passing by this row continue straight down.
    before.forEach((id, i) => {
      if (id && i !== lane) down.push({ from: i, to: i, color: colorOf(id), side: sides[i] });
    });
    // Lines to the parents: reuse a column already heading there, else the
    // node's own column for the first parent, else a free column.
    v.parents.filter((p) => known.has(p)).forEach((p, k) => {
      const side = tips.length > 0 && !(k === 0 && main.has(v.id) && main.has(p));
      let to = after.indexOf(p);
      if (to < 0) {
        to = k === 0 && after[lane] === null ? lane : after.indexOf(null);
        if (to < 0) to = after.length;
        after[to] = p;
        afterSides[to] = side;
        if (k === 0) colors.set(p, colors.get(p) ?? color);
      } else if (!side) afterSides[to] = false; // a main line takes the column over
      down.push({ from: lane, to, color: colorOf(p), side });
    });
    // Keep the main line left: if this column is now free and the first
    // parent is already reached through a column further right, pull that
    // column (and every line heading there) into this one.
    const first = v.parents.find((p) => known.has(p));
    const at = first ? after.indexOf(first) : -1;
    if (at > lane && after[lane] === null) {
      after[lane] = first!;
      after[at] = null;
      afterSides[lane] = afterSides[at];
      for (const s of down) if (s.to === at) s.to = lane;
    }

    while (after.length && after[after.length - 1] === null) after.pop();
    rows.push({ lane, color, down, width: Math.max(before.length, after.length, lane + 1) });
    lanes = after;
    sides = afterSides.slice(0, after.length);
  }
  return rows;
}
