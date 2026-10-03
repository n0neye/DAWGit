// Run with: npx esbuild src/lib/graph.test.ts --bundle --platform=node --outfile=%TEMP%/g.js && node %TEMP%/g.js
import { layout } from "./graph.js";
declare const process: { exitCode?: number };

function assert(cond: boolean, msg: string) {
  if (!cond) { console.error("FAIL:", msg); process.exitCode = 1; } else console.log("ok:", msg);
}

// Merge history from the real split test: merge(M) of drums(D) and group(G), both from base(B).
const rows = layout([
  { id: "M", parents: ["D", "G"] },
  { id: "G", parents: ["B"] },
  { id: "D", parents: ["B"] },
  { id: "B", parents: [] },
]);
assert(rows[0].lane === 0 && rows[0].down.length === 2, "merge has two lines down");
assert(rows[1].lane === 1, "second parent gets its own lane");
assert(rows[2].lane === 0, "first parent stays in lane 0");
assert(rows[3].lane === 0, "base back in lane 0");
assert(rows[2].down.some((s) => s.from === 1 && s.to === 0), "G's line curves back into lane 0");
assert(rows.every((r) => r.width <= 2), "never wider than 2 lanes");

// Linear history stays in one lane.
const linear = layout([{ id: "c", parents: ["b"] }, { id: "b", parents: ["a"] }, { id: "a", parents: [] }]);
assert(linear.every((r) => r.lane === 0 && r.width === 1), "linear history is one lane");

// A teammate's version merged in from the side: that line is dashed, the
// main line (first parents from the branch head) is not.
const side = layout([
  { id: "M", parents: ["C", "T"] },
  { id: "C", parents: ["X"] },
  { id: "T", parents: ["X"] },
  { id: "X", parents: [] },
], ["M"]);
const into = side[0].down.find((s) => s.to !== side[0].lane)!;
assert(!!into && into.side === true, "the line to the side parent is a side line");
assert(side[0].down.some((s) => s.to === side[0].lane && !s.side), "the first parent line is main");
assert(side[1].down.every((s) => (s.from === side[2].lane ? s.side : !s.side)), "the side column stays dashed past C");
assert(side[2].down.filter((s) => s.from === side[2].lane).every((s) => s.side), "T's line into X is a side line");
assert(layout([{ id: "M", parents: ["C", "T"] }, { id: "C", parents: [] }, { id: "T", parents: [] }]).every((r) => r.down.every((s) => !s.side)), "no tips: nothing dashed");
