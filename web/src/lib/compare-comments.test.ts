import { describe, it, expect } from "vitest";
import { partitionComments, sortCommentsByRenderOrder } from "./compare-comments";
import type { DiffHunk, ParsedDiff } from "./diff-parser";
import type { CommitFile, ReviewComment } from "@/types";

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

function makeHunk(lines: Array<{ old?: number | null; new?: number | null }>): DiffHunk {
  return {
    header: "",
    oldStart: 0,
    oldCount: lines.length,
    newStart: 0,
    newCount: lines.length,
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
    expect(got.anchored.map((c) => c.id)).toEqual(["c1"]);
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
    expect(got.caseB.map((c) => c.id)).toEqual(["cB"]);
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
    expect(partitionComments(diffs, totalLines, comments).anchored.map((c) => c.id)).toEqual(["l5"]);
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
    expect(partition.anchored.map((c) => c.id)).toEqual(["c1"]);
    expect(partition.caseB.map((c) => c.id)).toEqual(["cB"]);
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
