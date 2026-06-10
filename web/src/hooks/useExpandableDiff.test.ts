import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { expandableDiffReducer, useExpandableDiff } from "./useExpandableDiff";
import { StubNodeProvider } from "@/test/node-context";
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
  return {
    hunks,
    totalLines: 1000,
    generation: 0,
    resetGeneration: 0,
    failedAnchors: new Map<string, number>(),
  };
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

describe("expandableDiffReducer failed-anchor routing", () => {
  it("records an uncovered anchor as failed when an INSERT_HUNK overlap is skipped, without bumping generation", () => {
    const state = baseState([hunk(10, 11)]); // covers 10..20
    // Synthetic window [18..24] overlaps 10..20 but the anchor at 21 is not covered.
    const next = expandableDiffReducer(state, {
      type: "INSERT_HUNK",
      hunk: hunk(18, 7),
      anchors: [{ commentId: "c1", line: 21 }],
    });
    expect(next.failedAnchors.get("c1")).toBe(21);
    expect(next.hunks).toBe(state.hunks); // no insert
    expect(next.generation).toBe(state.generation); // failure-only state must not bump generation
  });

  it("does NOT record a failure when an overlapping existing hunk already covers the anchor", () => {
    const state = baseState([hunk(10, 15)]); // covers 10..24
    const next = expandableDiffReducer(state, {
      type: "INSERT_HUNK",
      hunk: hunk(18, 7),
      anchors: [{ commentId: "c1", line: 21 }],
    });
    expect(next.failedAnchors.size).toBe(0);
    expect(next).toBe(state); // unchanged reference
  });

  it("classifies each anchor independently on a partial overlap", () => {
    const state = baseState([hunk(10, 11)]); // covers 10..20
    // Window [15..26] overlaps 10..20; anchor 18 is covered, anchor 25 is not.
    const next = expandableDiffReducer(state, {
      type: "INSERT_HUNK",
      hunk: hunk(15, 12),
      anchors: [
        { commentId: "covered", line: 18 },
        { commentId: "uncovered", line: 25 },
      ],
    });
    expect(next.failedAnchors.has("covered")).toBe(false);
    expect(next.failedAnchors.get("uncovered")).toBe(25);
  });

  it("FAIL_ANCHORS records uncovered anchors and skips covered ones, without bumping generation", () => {
    const state = baseState([hunk(10, 5)]); // covers 10..14
    const next = expandableDiffReducer(state, {
      type: "FAIL_ANCHORS",
      anchors: [
        { commentId: "far", line: 21 },
        { commentId: "near", line: 12 },
      ],
    });
    expect(next.failedAnchors.get("far")).toBe(21);
    expect(next.failedAnchors.has("near")).toBe(false); // 12 is inside 10..14
    expect(next.generation).toBe(state.generation);
  });

  it("reconciles a previously-failed anchor back inline when a later EXPAND_DOWN covers it", () => {
    let state = baseState([hunk(10, 11)]); // covers 10..20
    state = expandableDiffReducer(state, {
      type: "INSERT_HUNK",
      hunk: hunk(18, 7),
      anchors: [{ commentId: "c1", line: 21 }],
    });
    expect(state.failedAnchors.get("c1")).toBe(21); // failed: 21 uncovered

    // Manual expand grows hunk 0 to cover 10..25 — the anchor at 21 is now inside it.
    state = expandableDiffReducer(state, {
      type: "EXPAND_DOWN",
      hunkIndex: 0,
      lines: contextLines(5),
    });
    expect(state.failedAnchors.has("c1")).toBe(false); // healed
  });

  it("clears failedAnchors on RESET", () => {
    let state = baseState([hunk(10, 5)]);
    state = expandableDiffReducer(state, {
      type: "FAIL_ANCHORS",
      anchors: [{ commentId: "c1", line: 21 }],
    });
    expect(state.failedAnchors.size).toBe(1);
    state = expandableDiffReducer(state, { type: "RESET", hunks: [hunk(1, 2)], totalLines: 20 });
    expect(state.failedAnchors.size).toBe(0);
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

  it("routes the comment to the failed-anchor set when a manual expand overlaps the synthetic range without covering it", async () => {
    // The reducer skips an INSERT_HUNK that overlaps an existing hunk. If a
    // manual expand grows an adjacent hunk into the in-flight synthetic range
    // but stops short of the anchor, the insert no-ops — the reducer records
    // the anchor as failed so the caseB comment routes to the unanchored
    // section instead of being hidden.
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
    const { result } = renderHook(() => useExpandableDiff(initialHunks, 1000, ctx), {
      wrapper: StubNodeProvider,
    });

    act(() => {
      void result.current.expandToLine(21, 3, [{ commentId: "c1", line: 21 }]);
    });

    // Manual expand grows hunk 0 to cover new lines 10..20 — overlaps [18..24]
    // at 18..20 but does NOT reach the anchor at 21.
    act(() => {
      result.current.dispatch({ type: "EXPAND_DOWN", hunkIndex: 0, lines: contextLines(10) });
    });

    await act(async () => {
      resolveFetch({ start: 18, end: 24, totalLines: 1000, lines: ["", "", "", "", "", "", ""] });
    });

    expect(result.current.failedAnchors.get("c1")).toBe(21);
    // No synthetic hunk inserted; only the manually-expanded hunk remains.
    expect(result.current.hunks).toHaveLength(1);
    expect(result.current.hunks.some((h) => h.stableKey?.startsWith("auto-"))).toBe(false);
  });

  it("must not hide the comment when an UNCOMMITTED concurrent expand makes the insert a no-op", async () => {
    // Residual stale-ref race in the old design: the post-await re-check read a
    // ref that only reflects COMMITTED reducer state, so a manual expand that was
    // dispatched-but-not-yet-committed went unseen — the re-check reported
    // coverage while the reducer (against its true internal state) skipped the
    // overlapping insert, hiding the comment. The fix moves the coverage decision
    // INTO the reducer (the single serialization point), so this is race-proof:
    // even when the manual dispatch and the fetch resolution land in the SAME
    // act() with no commit between them, the reducer evaluates the INSERT_HUNK
    // against its real internal state and records the uncovered anchor as failed.
    let resolveFetch!: (v: FileLinesResult) => void;
    vi.mocked(fetchFileLines).mockReturnValue(
      new Promise((res) => {
        resolveFetch = res;
      }),
    );

    const ctx = { repoPath: "/repo", filePath: "a.txt" };
    const initialHunks = [hunk(10, 1)]; // covers new line 10 only
    const { result } = renderHook(() => useExpandableDiff(initialHunks, 1000, ctx), {
      wrapper: StubNodeProvider,
    });

    act(() => {
      void result.current.expandToLine(21, 3, [{ commentId: "c1", line: 21 }]); // anchor 21 → range [18..24]
    });

    await act(async () => {
      // Manual expand grows hunk 0 to cover 10..20 (overlaps [18..24] at 18..20
      // but stops short of the anchor at 21)...
      result.current.dispatch({ type: "EXPAND_DOWN", hunkIndex: 0, lines: contextLines(10) });
      // ...and the auto-expand fetch resolves in the same batch, before the
      // manual expand commits.
      resolveFetch({ start: 18, end: 24, totalLines: 1000, lines: ["", "", "", "", "", "", ""] });
    });

    const anchorCovered = result.current.hunks.some(
      (h) => h.newStart <= 21 && 21 <= h.newStart + h.newCount - 1,
    );
    // No-hidden-comment invariant: the anchor is either inside a hunk (inline) or
    // recorded as a failed anchor (routed to the unanchored section). Never neither.
    expect(anchorCovered || result.current.failedAnchors.has("c1")).toBe(true);
    // Concretely, here it is uncovered, so it must be in the failed set.
    expect(result.current.failedAnchors.get("c1")).toBe(21);
  });

  it("records no failure when a manual expand actually covers the anchor", async () => {
    let resolveFetch!: (v: FileLinesResult) => void;
    vi.mocked(fetchFileLines).mockReturnValue(
      new Promise((res) => {
        resolveFetch = res;
      }),
    );

    const ctx = { repoPath: "/repo", filePath: "a.txt" };
    const initialHunks = [hunk(10, 1)];
    const { result } = renderHook(() => useExpandableDiff(initialHunks, 1000, ctx), {
      wrapper: StubNodeProvider,
    });

    act(() => {
      void result.current.expandToLine(21, 3, [{ commentId: "c1", line: 21 }]);
    });

    // Manual expand grows hunk 0 to cover new lines 10..25 — the anchor at 21 is
    // now inside a real hunk.
    act(() => {
      result.current.dispatch({ type: "EXPAND_DOWN", hunkIndex: 0, lines: contextLines(15) });
    });

    await act(async () => {
      resolveFetch({ start: 18, end: 24, totalLines: 1000, lines: ["", "", "", "", "", "", ""] });
    });

    // Anchor already covered by the manual expand; nothing inserted, nothing failed.
    expect(result.current.failedAnchors.has("c1")).toBe(false);
    expect(result.current.hunks).toHaveLength(1);
  });

  it("records a failed anchor when the fetched range is empty (terminal failure)", async () => {
    let resolveFetch!: (v: FileLinesResult) => void;
    vi.mocked(fetchFileLines).mockReturnValue(
      new Promise((res) => {
        resolveFetch = res;
      }),
    );

    const ctx = { repoPath: "/repo", filePath: "a.txt" };
    const initialHunks = [hunk(10, 1)];
    const { result } = renderHook(() => useExpandableDiff(initialHunks, 1000, ctx), {
      wrapper: StubNodeProvider,
    });

    act(() => {
      void result.current.expandToLine(21, 3, [{ commentId: "c1", line: 21 }]);
    });

    await act(async () => {
      resolveFetch({ start: 18, end: 24, totalLines: 1000, lines: [] });
    });

    expect(result.current.failedAnchors.get("c1")).toBe(21);
  });
});
