import { describe, it, expect } from "vitest";
import { sortCommentsByRenderOrder } from "./compare-comments";
import type { CompareFileView, CompareHunk } from "@/types";
import type { ReviewComment } from "@/types/review";

function makeFile(opts: Partial<CompareFileView> & { path: string; hunks: CompareHunk[] }): CompareFileView {
  return {
    status: "modified",
    additions: 0,
    deletions: 0,
    ...opts,
  };
}

function makeHunk(lines: Array<{ old?: number | null; new?: number | null }>): CompareHunk {
  return {
    kind: "diff",
    header: "",
    oldStart: 0,
    oldCount: lines.length,
    newStart: 0,
    newCount: lines.length,
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

describe("sortCommentsByRenderOrder", () => {
  it("returns [] for empty inputs", () => {
    expect(sortCommentsByRenderOrder([], [])).toEqual([]);
    expect(sortCommentsByRenderOrder([makeFile({ path: "a", hunks: [] })], [])).toEqual([]);
    expect(sortCommentsByRenderOrder([], [makeComment({ id: "x", file: "a", side: "R", line: 1 })])).toEqual([]);
  });

  it("orders comments across distinct files by file then line", () => {
    const files = [
      makeFile({ path: "a.txt", hunks: [makeHunk([{ new: 1 }, { new: 2 }])] }),
      makeFile({ path: "b.txt", hunks: [makeHunk([{ new: 1 }])] }),
    ];
    const comments = [
      makeComment({ id: "b1", file: "b.txt", side: "R", line: 1 }),
      makeComment({ id: "a2", file: "a.txt", side: "R", line: 2 }),
      makeComment({ id: "a1", file: "a.txt", side: "R", line: 1 }),
    ];
    const got = sortCommentsByRenderOrder(files, comments).map((c) => c.id);
    expect(got).toEqual(["a1", "a2", "b1"]);
  });

  it("orders comments in a single file by line position within hunks", () => {
    const files = [
      makeFile({ path: "a.txt", hunks: [makeHunk([{ new: 10 }, { new: 11 }, { new: 12 }])] }),
    ];
    const comments = [
      makeComment({ id: "c12", file: "a.txt", side: "R", line: 12 }),
      makeComment({ id: "c10", file: "a.txt", side: "R", line: 10 }),
      makeComment({ id: "c11", file: "a.txt", side: "R", line: 11 }),
    ];
    const got = sortCommentsByRenderOrder(files, comments).map((c) => c.id);
    expect(got).toEqual(["c10", "c11", "c12"]);
  });

  it("orders duplicate-path FileViews (real diff + synthetic snippet) by array position, not by line number", () => {
    // Real diff README.md with comment at R:30, then synthetic snippet README.md
    // with comment at R:46. Visual DOM: R:30 first (real, top of file), R:46
    // second (synthetic FileView below). Without the fix, R:46 sorts ahead
    // because the buggy old code looked up R:30 in the synthetic's posMap and
    // fell back to its raw line number.
    const files = [
      makeFile({ path: "README.md", hunks: [makeHunk([{ new: 30 }])] }),
      makeFile({
        path: "README.md",
        status: "context",
        hunks: [makeHunk([{ new: 46 }])],
      }),
    ];
    const comments = [
      makeComment({ id: "snippet46", file: "README.md", side: "R", line: 46 }),
      makeComment({ id: "real30", file: "README.md", side: "R", line: 30 }),
    ];
    const got = sortCommentsByRenderOrder(files, comments).map((c) => c.id);
    expect(got).toEqual(["real30", "snippet46"]);
  });

  it("matches L-side comments on renamed files via oldPath", () => {
    const files = [
      makeFile({
        path: "new.txt",
        oldPath: "old.txt",
        status: "renamed",
        hunks: [makeHunk([{ old: 5, new: null }])],
      }),
    ];
    const comments = [
      makeComment({ id: "l5", file: "new.txt", oldPath: "old.txt", side: "L", line: 5 }),
    ];
    expect(sortCommentsByRenderOrder(files, comments).map((c) => c.id)).toEqual(["l5"]);
  });

  it("emits a comment exactly once when its anchor line appears in multiple hunks", () => {
    const files = [
      makeFile({
        path: "a.txt",
        hunks: [
          makeHunk([{ new: 5 }]),
          makeHunk([{ new: 5 }]), // same anchor in a second hunk (e.g. duplicated context)
        ],
      }),
    ];
    const comments = [makeComment({ id: "dup", file: "a.txt", side: "R", line: 5 })];
    expect(sortCommentsByRenderOrder(files, comments).map((c) => c.id)).toEqual(["dup"]);
  });

  it("does not match an L-side comment against a different file's new path", () => {
    // Two unrelated files; comment is L-side on old.txt. Must not match new.txt.
    const files = [
      makeFile({ path: "new.txt", hunks: [makeHunk([{ old: 5, new: null }])] }),
    ];
    const comments = [
      makeComment({ id: "stray", file: "new.txt", oldPath: "old.txt", side: "L", line: 5 }),
    ];
    expect(sortCommentsByRenderOrder(files, comments)).toEqual([]);
  });
});
