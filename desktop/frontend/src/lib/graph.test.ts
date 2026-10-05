import { describe, expect, it } from "vitest";
import { layout } from "./graph";

describe("history graph", () => {
  // Merge history from the real split test: merge(M) of drums(D) and group(G), both from base(B).
  const rows = layout([
    { id: "M", parents: ["D", "G"] },
    { id: "G", parents: ["B"] },
    { id: "D", parents: ["B"] },
    { id: "B", parents: [] },
  ]);
  it("gives a merge two lines down", () => expect(rows[0].down.length).toBe(2));
  it("puts the second parent in its own lane", () => expect(rows[1].lane).toBe(1));
  it("keeps the first parent in lane 0", () => {
    expect(rows[0].lane).toBe(0);
    expect(rows[2].lane).toBe(0);
    expect(rows[3].lane).toBe(0);
  });
  it("curves the side line back into lane 0", () =>
    expect(rows[2].down.some((s) => s.from === 1 && s.to === 0)).toBe(true));
  it("is never wider than 2 lanes", () => expect(rows.every((r) => r.width <= 2)).toBe(true));
});

describe("history graph lines", () => {
  it("keeps linear history in one lane", () => {
    const linear = layout([{ id: "c", parents: ["b"] }, { id: "b", parents: ["a"] }, { id: "a", parents: [] }]);
    expect(linear.every((r) => r.lane === 0 && r.width === 1)).toBe(true);
  });

  // A teammate's version merged in from the side: that line is dashed, the
  // main line (first parents from the branch head) is not.
  const side = layout([
    { id: "M", parents: ["C", "T"] },
    { id: "C", parents: ["X"] },
    { id: "T", parents: ["X"] },
    { id: "X", parents: [] },
  ], ["M"]);
  it("dashes the line to the side parent", () => {
    const into = side[0].down.find((s) => s.to !== side[0].lane);
    expect(into?.side).toBe(true);
    expect(side[0].down.some((s) => s.to === side[0].lane && !s.side)).toBe(true);
  });
  it("keeps the side column dashed past C", () =>
    expect(side[1].down.every((s) => (s.from === side[2].lane ? s.side : !s.side))).toBe(true));
  it("dashes T's line into X", () =>
    expect(side[2].down.filter((s) => s.from === side[2].lane).every((s) => s.side)).toBe(true));
  it("dashes nothing without tips", () =>
    expect(layout([{ id: "M", parents: ["C", "T"] }, { id: "C", parents: [] }, { id: "T", parents: [] }])
      .every((r) => r.down.every((s) => !s.side))).toBe(true));
});
