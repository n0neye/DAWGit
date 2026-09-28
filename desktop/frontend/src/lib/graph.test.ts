// Run with: npm run test:graph
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
