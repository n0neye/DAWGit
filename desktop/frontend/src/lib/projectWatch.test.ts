import { describe, expect, it } from "vitest";
import { settler } from "./projectWatch.svelte";

describe("settler", () => {
  it("says so once a changed file stops changing", () => {
    const settle = settler();
    expect(settle("a")).toBe(false); // where it starts
    expect(settle("a")).toBe(false);
    expect(settle("b")).toBe(false); // changed: still being written?
    expect(settle("c")).toBe(false); // still changing
    expect(settle("c")).toBe(true); // the same twice: done
    expect(settle("c")).toBe(false); // and only once
  });
});
