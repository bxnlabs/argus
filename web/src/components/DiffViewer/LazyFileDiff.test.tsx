import { describe, it, expect } from "vitest";
import { render } from "@testing-library/react";
import {
  FilePlaceholder,
  FILE_HEADER_HEIGHT_PX,
  PLACEHOLDER_LINE_HEIGHT_PX,
  HUNK_OVERHEAD_PX,
} from "./LazyFileDiff";
import type { ParsedDiff, DiffHunk, DiffLine } from "@/lib/diff-parser";

function makeDiff(overrides: Partial<ParsedDiff> = {}): ParsedDiff {
  return {
    oldFile: "a.txt",
    newFile: "a.txt",
    isBinary: false,
    isNew: false,
    isDeleted: false,
    isRenamed: false,
    hunks: [],
    additions: 0,
    deletions: 0,
    ...overrides,
  };
}

function makeHunk(lineCount: number): DiffHunk {
  const lines: DiffLine[] = Array.from({ length: lineCount }, (_, i) => ({
    type: "context",
    content: `line ${i}`,
    oldLineNumber: i + 1,
    newLineNumber: i + 1,
  }));
  return {
    header: "@@ -1,1 +1,1 @@",
    oldStart: 1,
    oldCount: lineCount,
    newStart: 1,
    newCount: lineCount,
    lines,
    stableKey: `hunk-${lineCount}`,
  };
}

describe("FilePlaceholder", () => {
  it("reserves just the file header when there are no hunks", () => {
    const { container } = render(
      <FilePlaceholder diff={makeDiff()} fileName="a.txt" />,
    );
    const el = container.firstElementChild as HTMLElement;
    expect(el.style.minHeight).toBe(`${FILE_HEADER_HEIGHT_PX}px`);
  });

  it("sums DiffLine rows across hunks and per-hunk overhead", () => {
    const diff = makeDiff({ hunks: [makeHunk(10), makeHunk(5)] });
    const { container } = render(
      <FilePlaceholder diff={diff} fileName="a.txt" />,
    );
    const el = container.firstElementChild as HTMLElement;
    const expected =
      FILE_HEADER_HEIGHT_PX +
      15 * PLACEHOLDER_LINE_HEIGHT_PX +
      2 * HUNK_OVERHEAD_PX;
    expect(el.style.minHeight).toBe(`${expected}px`);
  });

  it("handles a single-hunk diff", () => {
    const diff = makeDiff({ hunks: [makeHunk(3)] });
    const { container } = render(
      <FilePlaceholder diff={diff} fileName="a.txt" />,
    );
    const el = container.firstElementChild as HTMLElement;
    const expected =
      FILE_HEADER_HEIGHT_PX + 3 * PLACEHOLDER_LINE_HEIGHT_PX + HUNK_OVERHEAD_PX;
    expect(el.style.minHeight).toBe(`${expected}px`);
  });

  it("uses the published constants (44px header, 20px per line, 40px per hunk)", () => {
    expect(FILE_HEADER_HEIGHT_PX).toBe(44);
    expect(PLACEHOLDER_LINE_HEIGHT_PX).toBe(20);
    expect(HUNK_OVERHEAD_PX).toBe(40);
  });
});
