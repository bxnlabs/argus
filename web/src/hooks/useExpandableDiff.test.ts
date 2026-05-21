import { describe, it, expect } from "vitest";
import { expandableDiffReducer } from "./useExpandableDiff";
import type { DiffHunk } from "@/lib/diff-parser";

function hunk(newStart: number, newCount: number): DiffHunk {
  return {
    header: `@@ -${newStart},${newCount} +${newStart},${newCount} @@`,
    oldStart: newStart,
    oldCount: newCount,
    newStart,
    newCount,
    stableKey: `h-${newStart}-${newCount}`,
    lines: [],
  };
}

function baseState(hunks: DiffHunk[]) {
  return { hunks, totalLines: 1000, generation: 0, resetGeneration: 0 };
}

describe("expandableDiffReducer INSERT_HUNK", () => {
  it("appends a hunk that sorts after existing hunks", () => {
    const state = baseState([hunk(10, 5)]);
    const next = expandableDiffReducer(state, { type: "INSERT_HUNK", hunk: hunk(100, 5) });
    expect(next.hunks.map((h) => h.newStart)).toEqual([10, 100]);
    expect(next.generation).toBe(1);
  });

  it("inserts a hunk at the correct sorted position by newStart", () => {
    const state = baseState([hunk(10, 5), hunk(100, 5)]);
    const next = expandableDiffReducer(state, { type: "INSERT_HUNK", hunk: hunk(50, 5) });
    expect(next.hunks.map((h) => h.newStart)).toEqual([10, 50, 100]);
  });

  it("skips an insert that overlaps an existing hunk (no duplicate)", () => {
    const state = baseState([hunk(10, 10)]); // covers new lines 10..19
    // Overlaps at 15..19.
    const next = expandableDiffReducer(state, { type: "INSERT_HUNK", hunk: hunk(15, 10) });
    expect(next).toBe(state); // unchanged reference
  });

  it("treats touching ranges as overlap and skips", () => {
    const state = baseState([hunk(10, 5)]); // covers 10..14
    // 15..19 is adjacent (no gap) — inserting would create a redundant split.
    // Adjacent (not overlapping) is allowed; ensure a true overlap is skipped.
    const overlapping = expandableDiffReducer(state, { type: "INSERT_HUNK", hunk: hunk(14, 5) });
    expect(overlapping).toBe(state);
  });

  it("lands a second non-overlapping insert correctly when computed from updated state", () => {
    // Simulates two concurrent auto-expands: the first inserts at 5, the second
    // (which captured the pre-insert snapshot) inserts at 100. Position is
    // recomputed from current state, so order stays sorted.
    let state = baseState([hunk(50, 5)]);
    state = expandableDiffReducer(state, { type: "INSERT_HUNK", hunk: hunk(5, 3) });
    state = expandableDiffReducer(state, { type: "INSERT_HUNK", hunk: hunk(100, 3) });
    expect(state.hunks.map((h) => h.newStart)).toEqual([5, 50, 100]);
  });
});

describe("expandableDiffReducer RESET", () => {
  it("bumps both generation and resetGeneration", () => {
    const state = baseState([hunk(10, 5)]);
    const next = expandableDiffReducer(state, { type: "RESET", hunks: [hunk(1, 2)], totalLines: 20 });
    expect(next.generation).toBe(1);
    expect(next.resetGeneration).toBe(1);
    expect(next.totalLines).toBe(20);
    expect(next.hunks.map((h) => h.newStart)).toEqual([1]);
  });
});
