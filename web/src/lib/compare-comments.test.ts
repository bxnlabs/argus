import { describe, it, expect } from "vitest";
import { partitionComments, sortCommentsByRenderOrder, coalesceAutoExpand } from "./compare-comments";
import type { DiffHunk, ParsedDiff } from "./diff-parser";
import type { AnchorStatus, CommitFile, ReviewComment } from "@/types";

function makeFile(
  opts: Partial<ParsedDiff> & { path: string; hunks: DiffHunk[]; oldPath?: string },
): ParsedDiff {
  const newFile = opts.path;
  const oldFile = opts.oldPath ?? opts.path;
  return {
    oldFile,
    newFile,
    additions: 0,
    deletions: 0,
    isBinary: false,
    isNew: false,
    isDeleted: false,
    isRenamed: !!opts.oldPath && opts.oldPath !== opts.path,
    hunks: opts.hunks,
  };
}

function makeHunk(
  lines: Array<{ old?: number | null; new?: number | null }>,
  bounds?: { oldStart: number; oldCount: number; newStart: number; newCount: number },
): DiffHunk {
  return {
    header: "",
    oldStart: bounds?.oldStart ?? 0,
    oldCount: bounds?.oldCount ?? lines.length,
    newStart: bounds?.newStart ?? 0,
    newCount: bounds?.newCount ?? lines.length,
    stableKey: "test",
    lines: lines.map((l) => ({
      type: "context",
      content: "",
      oldLineNumber: l.old ?? null,
      newLineNumber: l.new ?? null,
    })),
  };
}

function makeComment(opts: {
  id: string;
  file: string;
  oldPath?: string;
  side: "L" | "R";
  line: number;
  anchorStatus?: AnchorStatus;
}): ReviewComment {
  return {
    id: opts.id,
    file: opts.file,
    oldPath: opts.oldPath,
    line: {
      from: { side: opts.side, line: opts.line },
      to: { side: opts.side, line: opts.line },
    },
    snippet: "",
    anchorStatus: opts.anchorStatus,
    body: "",
    submitted: true,
    createdAt: "2026-05-17T00:00:00Z",
  };
}

function makeCommitFile(path: string, oldPath?: string): CommitFile {
  return {
    path,
    oldPath,
    status: oldPath && oldPath !== path ? "renamed" : "modified",
    additions: 0,
    deletions: 0,
  };
}

describe("partitionComments", () => {
  it("classifies an anchored comment when the line is present in a hunk", () => {
    const diffs = [makeFile({ path: "a.txt", hunks: [makeHunk([{ new: 1 }, { new: 2 }])] })];
    const files = [makeCommitFile("a.txt")];
    const totalLines = { "a.txt": 100 };
    const comments = [makeComment({ id: "c1", file: "a.txt", side: "R", line: 1 })];
    const got = partitionComments(diffs, totalLines, comments);
    expect(got.anchored.map((e) => e.comment.id)).toEqual(["c1"]);
    expect(got.caseB).toEqual([]);
    expect(got.unanchored).toEqual([]);
  });

  it("classifies as caseB when file is present but the line is outside any hunk", () => {
    const diffs = [makeFile({ path: "a.txt", hunks: [makeHunk([{ new: 1 }, { new: 2 }])] })];
    const files = [makeCommitFile("a.txt")];
    const totalLines = { "a.txt": 100 };
    const comments = [makeComment({ id: "cB", file: "a.txt", side: "R", line: 50 })];
    const got = partitionComments(diffs, totalLines, comments);
    expect(got.anchored).toEqual([]);
    expect(got.caseB.map((e) => e.comment.id)).toEqual(["cB"]);
    // R-side anchor expands around its own line (no translation).
    expect(got.caseB[0].autoExpandLine).toBe(50);
    expect(got.unanchored).toEqual([]);
  });

  it("classifies as unanchored when the line is beyond totalLines", () => {
    const diffs = [makeFile({ path: "a.txt", hunks: [makeHunk([{ new: 1 }])] })];
    const files = [makeCommitFile("a.txt")];
    const totalLines = { "a.txt": 10 };
    const comments = [makeComment({ id: "off", file: "a.txt", side: "R", line: 999 })];
    const got = partitionComments(diffs, totalLines, comments);
    expect(got.unanchored.map((c) => c.id)).toEqual(["off"]);
    expect(got.caseB).toEqual([]);
    expect(got.anchored).toEqual([]);
  });

  it("classifies as unanchored when the file isn't in any parsed diff", () => {
    const diffs = [makeFile({ path: "a.txt", hunks: [makeHunk([{ new: 1 }])] })];
    const files = [makeCommitFile("a.txt")];
    const totalLines = { "a.txt": 100 };
    const comments = [makeComment({ id: "miss", file: "other.txt", side: "R", line: 5 })];
    const got = partitionComments(diffs, totalLines, comments);
    expect(got.unanchored.map((c) => c.id)).toEqual(["miss"]);
  });

  it("classifies as unanchored when totalLines is missing for the file", () => {
    const diffs = [makeFile({ path: "a.txt", hunks: [makeHunk([{ new: 1 }])] })];
    const files = [makeCommitFile("a.txt")];
    const totalLines: Record<string, number> = {};
    const comments = [makeComment({ id: "no-total", file: "a.txt", side: "R", line: 5 })];
    const got = partitionComments(diffs, totalLines, comments);
    expect(got.unanchored.map((c) => c.id)).toEqual(["no-total"]);
  });

  it("matches L-side comments on renamed files via oldPath", () => {
    const diffs = [
      makeFile({
        path: "new.txt",
        oldPath: "old.txt",
        hunks: [makeHunk([{ old: 5, new: null }])],
      }),
    ];
    const files = [makeCommitFile("new.txt", "old.txt")];
    const totalLines = { "new.txt": 100 };
    const comments = [
      makeComment({ id: "l5", file: "new.txt", oldPath: "old.txt", side: "L", line: 5 }),
    ];
    expect(
      partitionComments(diffs, totalLines, comments).anchored.map((e) => e.comment.id),
    ).toEqual(["l5"]);
  });

  it("routes a backend-marked unanchored comment to the unanchored bucket", () => {
    // File and line would otherwise classify as anchored, but the backend
    // determined the snippet/file is gone — it must go to read/prune, not inline.
    const diffs = [makeFile({ path: "a.txt", hunks: [makeHunk([{ new: 1 }])] })];
    const totalLines = { "a.txt": 100 };
    const comments = [
      makeComment({ id: "gone", file: "a.txt", side: "R", line: 1, anchorStatus: "unanchored" }),
    ];
    const got = partitionComments(diffs, totalLines, comments);
    expect(got.unanchored.map((c) => c.id)).toEqual(["gone"]);
    expect(got.anchored).toEqual([]);
    expect(got.caseB).toEqual([]);
  });

  it("translates an L-side caseB anchor to the new side before the range check", () => {
    // Hunk adds 3 lines near the top: old 1-2 → new 1-5. An L-side comment on
    // old line 50 (outside the hunk) translates to new line 53.
    const hunk = makeHunk(
      [
        { old: 1, new: 1 },
        { old: 2, new: 2 },
        { old: null, new: 3 },
        { old: null, new: 4 },
        { old: null, new: 5 },
      ],
      { oldStart: 1, oldCount: 2, newStart: 1, newCount: 5 },
    );
    const diffs = [makeFile({ path: "a.txt", hunks: [hunk] })];
    const totalLines = { "a.txt": 100 };
    const comments = [makeComment({ id: "lB", file: "a.txt", side: "L", line: 50 })];
    const got = partitionComments(diffs, totalLines, comments);
    expect(got.caseB.map((e) => e.comment.id)).toEqual(["lB"]);
    expect(got.caseB[0].autoExpandLine).toBe(53);
  });

  it("marks an L-side comment unanchored when its translated line exceeds the new file", () => {
    // Same hunk (old 1-2 → new 1-5). Old line 200 translates to new 203, which
    // is beyond the 100-line new file → unanchored, not caseB.
    const hunk = makeHunk(
      [
        { old: 1, new: 1 },
        { old: 2, new: 2 },
        { old: null, new: 3 },
        { old: null, new: 4 },
        { old: null, new: 5 },
      ],
      { oldStart: 1, oldCount: 2, newStart: 1, newCount: 5 },
    );
    const diffs = [makeFile({ path: "a.txt", hunks: [hunk] })];
    const totalLines = { "a.txt": 100 };
    const comments = [makeComment({ id: "lOff", file: "a.txt", side: "L", line: 200 })];
    const got = partitionComments(diffs, totalLines, comments);
    expect(got.unanchored.map((c) => c.id)).toEqual(["lOff"]);
    expect(got.caseB).toEqual([]);
  });

  it("doesn't match an L-side comment against a different file's new path", () => {
    const diffs = [
      makeFile({ path: "new.txt", hunks: [makeHunk([{ old: 5, new: null }])] }),
    ];
    const files = [makeCommitFile("new.txt")];
    const totalLines = { "new.txt": 100 };
    const comments = [
      makeComment({ id: "stray", file: "new.txt", oldPath: "old.txt", side: "L", line: 5 }),
    ];
    const got = partitionComments(diffs, totalLines, comments);
    // No matching parsed diff (oldFile is "new.txt", not "old.txt") → unanchored.
    expect(got.unanchored.map((c) => c.id)).toEqual(["stray"]);
  });
});

describe("sortCommentsByRenderOrder", () => {
  it("returns [] for empty inputs", () => {
    expect(
      sortCommentsByRenderOrder([], [], { anchored: [], caseB: [], unanchored: [] }),
    ).toEqual([]);
  });

  it("orders anchored comments across files by file then line", () => {
    const diffs = [
      makeFile({ path: "a.txt", hunks: [makeHunk([{ new: 1 }, { new: 2 }])] }),
      makeFile({ path: "b.txt", hunks: [makeHunk([{ new: 1 }])] }),
    ];
    const files = [makeCommitFile("a.txt"), makeCommitFile("b.txt")];
    const totalLines = { "a.txt": 100, "b.txt": 100 };
    const comments = [
      makeComment({ id: "b1", file: "b.txt", side: "R", line: 1 }),
      makeComment({ id: "a2", file: "a.txt", side: "R", line: 2 }),
      makeComment({ id: "a1", file: "a.txt", side: "R", line: 1 }),
    ];
    const partition = partitionComments(diffs, totalLines, comments);
    const got = sortCommentsByRenderOrder(diffs, files, partition).map((c) => c.id);
    expect(got).toEqual(["a1", "a2", "b1"]);
  });

  it("interleaves anchored and caseB comments per file by line", () => {
    // File has hunk lines at 1, 2 only; line 50 is caseB.
    const diffs = [
      makeFile({ path: "a.txt", hunks: [makeHunk([{ new: 1 }, { new: 2 }])] }),
    ];
    const files = [makeCommitFile("a.txt")];
    const totalLines = { "a.txt": 100 };
    const comments = [
      makeComment({ id: "cB", file: "a.txt", side: "R", line: 50 }),
      makeComment({ id: "c1", file: "a.txt", side: "R", line: 1 }),
    ];
    const partition = partitionComments(diffs, totalLines, comments);
    expect(partition.anchored.map((e) => e.comment.id)).toEqual(["c1"]);
    expect(partition.caseB.map((e) => e.comment.id)).toEqual(["cB"]);
    const got = sortCommentsByRenderOrder(diffs, files, partition).map((c) => c.id);
    expect(got).toEqual(["c1", "cB"]);
  });

  it("appends unanchored comments after inline ones, grouped by file order", () => {
    const diffs = [
      makeFile({ path: "a.txt", hunks: [makeHunk([{ new: 1 }])] }),
    ];
    const files = [makeCommitFile("a.txt"), makeCommitFile("b.txt")];
    const totalLines = { "a.txt": 10, "b.txt": 10 };
    const comments = [
      // b.txt isn't in parsedDiffs → unanchored
      makeComment({ id: "b1", file: "b.txt", side: "R", line: 5 }),
      makeComment({ id: "b2", file: "b.txt", side: "R", line: 2 }),
      // a.txt anchored line 1
      makeComment({ id: "a1", file: "a.txt", side: "R", line: 1 }),
      // a.txt anchor past EOF → unanchored
      makeComment({ id: "a999", file: "a.txt", side: "R", line: 999 }),
    ];
    const partition = partitionComments(diffs, totalLines, comments);
    const got = sortCommentsByRenderOrder(diffs, files, partition).map((c) => c.id);
    // a1 (anchored) first; then unanchored grouped by file in files-order
    // — a.txt first (since it appears in files), then b.txt; within each
    // file sorted by line.
    expect(got).toEqual(["a1", "a999", "b2", "b1"]);
  });

  it("orders mixed L-side and R-side comments by new-side render position", () => {
    // Hunk adds lines near the top (old 1-2 → new 1-22), so an old-side anchor
    // past the hunk maps to a much larger new-side line. An L-side comment on
    // old line 5 renders at new line 25 — BELOW an R-side comment at new line
    // 10. Sorting by raw line.from.line (5 vs 10) would wrongly put L first.
    const hunk = makeHunk(
      [
        { old: 1, new: 1 },
        { old: 2, new: 2 },
        { old: null, new: 3 },
      ],
      { oldStart: 1, oldCount: 2, newStart: 1, newCount: 22 },
    );
    const diffs = [makeFile({ path: "a.txt", hunks: [hunk] })];
    const files = [makeCommitFile("a.txt")];
    const totalLines = { "a.txt": 100 };
    const comments = [
      makeComment({ id: "lOld5", file: "a.txt", side: "L", line: 5 }),
      makeComment({ id: "rNew10", file: "a.txt", side: "R", line: 10 }),
    ];
    const partition = partitionComments(diffs, totalLines, comments);
    // Both land in caseB (neither anchor is among the listed hunk lines).
    expect(partition.caseB.map((e) => e.comment.id).sort()).toEqual(["lOld5", "rNew10"]);
    const got = sortCommentsByRenderOrder(diffs, files, partition).map((c) => c.id);
    expect(got).toEqual(["rNew10", "lOld5"]);
  });

  it("breaks new-side ties L-before-R (deletion renders above its paired addition)", () => {
    // A replace hunk: old line 5 deleted, new line 5 added. The L-side comment
    // on old 5 and the R-side comment on new 5 both resolve to new-side line 5,
    // so the sort key ties. The deletion row renders above the addition row in a
    // unified diff, so nav order must be L then R regardless of input order.
    const hunk = makeHunk(
      [
        { old: 4, new: 4 },
        { old: 5, new: null },
        { old: null, new: 5 },
        { old: 6, new: 6 },
      ],
      { oldStart: 4, oldCount: 3, newStart: 4, newCount: 3 },
    );
    const diffs = [makeFile({ path: "a.txt", hunks: [hunk] })];
    const files = [makeCommitFile("a.txt")];
    const totalLines = { "a.txt": 100 };
    // Authored R-before-L so a stable sort without a tie-breaker would keep R first.
    const comments = [
      makeComment({ id: "rAdd5", file: "a.txt", side: "R", line: 5 }),
      makeComment({ id: "lDel5", file: "a.txt", side: "L", line: 5 }),
    ];
    const partition = partitionComments(diffs, totalLines, comments);
    // Both are anchored (their lines exist in the hunk on their respective sides).
    expect(partition.anchored.map((e) => e.comment.id)).toEqual(["rAdd5", "lDel5"]);
    const got = sortCommentsByRenderOrder(diffs, files, partition).map((c) => c.id);
    expect(got).toEqual(["lDel5", "rAdd5"]);
  });

  it("appends unanchored comments for files not in compareData.files last", () => {
    const diffs: ParsedDiff[] = [];
    const files: CommitFile[] = [];
    const comments = [
      makeComment({ id: "z", file: "zzz.txt", side: "R", line: 1 }),
      makeComment({ id: "a", file: "aaa.txt", side: "R", line: 1 }),
    ];
    const partition = partitionComments(diffs, {}, comments);
    expect(partition.unanchored).toHaveLength(2);
    const got = sortCommentsByRenderOrder(diffs, files, partition).map((c) => c.id);
    // First-encounter order in the comments array: z then a.
    expect(got).toEqual(["z", "a"]);
  });
});

describe("coalesceAutoExpand", () => {
  it("returns [] for no anchors", () => {
    expect(coalesceAutoExpand([], 3)).toEqual([]);
  });

  it("keeps a single anchor as its own window", () => {
    const got = coalesceAutoExpand([{ line: 50, commentId: "a" }], 3);
    expect(got).toEqual([{ line: 50, radius: 3, commentIds: ["a"] }]);
  });

  it("merges anchors whose windows overlap into one target", () => {
    // 50±3 = [47,53] and 56±3 = [53,59] overlap at 53 → merge to [47,59].
    const got = coalesceAutoExpand(
      [
        { line: 50, commentId: "a" },
        { line: 56, commentId: "b" },
      ],
      3,
    );
    expect(got).toHaveLength(1);
    expect(got[0].commentIds).toEqual(["a", "b"]);
    // Merged window [47,59] → center 53, radius 6 covers both anchors.
    expect(got[0].line - got[0].radius).toBeLessThanOrEqual(47);
    expect(got[0].line + got[0].radius).toBeGreaterThanOrEqual(59);
  });

  it("keeps far-apart anchors as separate targets", () => {
    const got = coalesceAutoExpand(
      [
        { line: 50, commentId: "a" },
        { line: 500, commentId: "b" },
      ],
      3,
    );
    expect(got).toHaveLength(2);
    expect(got.map((t) => t.commentIds)).toEqual([["a"], ["b"]]);
  });

  it("sorts unsorted anchors before merging", () => {
    const got = coalesceAutoExpand(
      [
        { line: 56, commentId: "b" },
        { line: 50, commentId: "a" },
      ],
      3,
    );
    expect(got).toHaveLength(1);
    expect(got[0].commentIds).toEqual(["a", "b"]);
  });

  it("never centers a coalesced window inside an existing hunk", () => {
    // A pure deletion leaves a new-side hunk covering [11,16] between two caseB
    // anchors at 10 and 17 (gap 7 — within the merge threshold of 2*radius+1).
    // If they coalesce, the window centers at 13 (inside the hunk), so
    // expandToLine's center-in-hunk early return fires and BOTH comments
    // silently vanish (neither inline nor in the unanchored section). The
    // window(s) must keep their centers out of the hunk so each anchor stays
    // reachable.
    const hunks = [{ newStart: 11, newCount: 6 }]; // covers new lines 11..16
    const got = coalesceAutoExpand(
      [
        { line: 10, commentId: "a" },
        { line: 17, commentId: "b" },
      ],
      3,
      hunks,
    );
    // No comment is dropped.
    expect(got.flatMap((t) => t.commentIds).sort()).toEqual(["a", "b"]);
    // No window centers inside the real hunk.
    for (const t of got) {
      expect(t.line < 11 || t.line > 16).toBe(true);
    }
  });
});
