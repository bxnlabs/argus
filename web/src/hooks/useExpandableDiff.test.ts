import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { expandableDiffReducer, useExpandableDiff } from "./useExpandableDiff";
import type { DiffHunk, DiffLine } from "@/lib/diff-parser";

vi.mock("@/data/git/file-lines", () => ({
  fetchFileLines: vi.fn(),
}));
import { fetchFileLines, type FileLinesResult } from "@/data/git/file-lines";

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

function contextLines(n: number): DiffLine[] {
  return Array.from({ length: n }, () => ({
    type: "context" as const,
    content: "x",
    oldLineNumber: null,
    newLineNumber: null,
  }));
}

describe("useExpandableDiff expandToLine vs concurrent manual expand", () => {
  beforeEach(() => {
    vi.mocked(fetchFileLines).mockReset();
  });

  it("returns false when a manual expand overlaps the synthetic range without covering the anchor", async () => {
    // The reducer skips an INSERT_HUNK that overlaps an existing hunk. If a
    // manual expand grows an adjacent hunk into the in-flight synthetic range
    // but stops short of the anchor, the insert no-ops — expandToLine must
    // report false so the caseB comment routes to the unanchored section
    // instead of being hidden.
    let resolveFetch!: (v: FileLinesResult) => void;
    vi.mocked(fetchFileLines).mockReturnValue(
      new Promise((res) => {
        resolveFetch = res;
      }),
    );

    const ctx = { repoPath: "/repo", filePath: "a.txt" };
    // Stable references: the hook resets when `initialHunks`/`totalLines` change
    // by identity, so a fresh array per render would loop forever.
    const initialHunks = [hunk(10, 1)];
    // One hunk covering new line 10 only; caseB anchor at 21 → clamps to [18..24].
    const { result } = renderHook(() => useExpandableDiff(initialHunks, 1000, ctx));

    let expandPromise!: Promise<boolean>;
    act(() => {
      expandPromise = result.current.expandToLine(21, 3);
    });

    // Manual expand grows hunk 0 to cover new lines 10..20 — overlaps [18..24]
    // at 18..20 but does NOT reach the anchor at 21.
    act(() => {
      result.current.dispatch({ type: "EXPAND_DOWN", hunkIndex: 0, lines: contextLines(10) });
    });

    let covered: boolean | undefined;
    await act(async () => {
      resolveFetch({ start: 18, end: 24, totalLines: 1000, lines: ["", "", "", "", "", "", ""] });
      covered = await expandPromise;
    });

    expect(covered).toBe(false);
    // No synthetic hunk inserted; only the manually-expanded hunk remains.
    expect(result.current.hunks).toHaveLength(1);
    expect(result.current.hunks.some((h) => h.stableKey?.startsWith("auto-"))).toBe(false);
  });

  it("returns true when a manual expand actually covers the anchor", async () => {
    let resolveFetch!: (v: FileLinesResult) => void;
    vi.mocked(fetchFileLines).mockReturnValue(
      new Promise((res) => {
        resolveFetch = res;
      }),
    );

    const ctx = { repoPath: "/repo", filePath: "a.txt" };
    const initialHunks = [hunk(10, 1)];
    const { result } = renderHook(() => useExpandableDiff(initialHunks, 1000, ctx));

    let expandPromise!: Promise<boolean>;
    act(() => {
      expandPromise = result.current.expandToLine(21, 3);
    });

    // Manual expand grows hunk 0 to cover new lines 10..25 — the anchor at 21 is
    // now inside a real hunk.
    act(() => {
      result.current.dispatch({ type: "EXPAND_DOWN", hunkIndex: 0, lines: contextLines(15) });
    });

    let covered: boolean | undefined;
    await act(async () => {
      resolveFetch({ start: 18, end: 24, totalLines: 1000, lines: ["", "", "", "", "", "", ""] });
      covered = await expandPromise;
    });

    expect(covered).toBe(true);
    // Anchor already covered by the manual expand; no synthetic hunk needed.
    expect(result.current.hunks).toHaveLength(1);
  });
});
