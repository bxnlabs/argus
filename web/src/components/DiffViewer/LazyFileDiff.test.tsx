import { describe, it, expect } from "vitest";
import { render } from "@testing-library/react";
import { FilePlaceholder, estimatePlaceholderMinHeight } from "./LazyFileDiff";
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

function makeHunk(
  lineCount: number,
  opts: {
    withHeaderLine?: boolean;
    contentLength?: number;
    newStart?: number;
    newCount?: number;
  } = {},
): DiffHunk {
  const content = "x".repeat(opts.contentLength ?? 4);
  const newStart = opts.newStart ?? 1;
  const newCount = opts.newCount ?? lineCount;
  const lines: DiffLine[] = Array.from({ length: lineCount }, (_, i) => ({
    type: "context",
    content,
    oldLineNumber: i + newStart,
    newLineNumber: i + newStart,
  }));
  if (opts.withHeaderLine) {
    lines.unshift({
      type: "header",
      content: "fn section()",
      oldLineNumber: null,
      newLineNumber: null,
    });
  }
  return {
    header: "@@ ... @@",
    oldStart: newStart,
    oldCount: newCount,
    newStart,
    newCount,
    lines,
    stableKey: `hunk-${newStart}-${lineCount}`,
  };
}

// Single hunk covering its whole "file" (newStart=1, reaches totalLines), so no
// expand rows render — a clean baseline for isolating one variable at a time.
function wholeFileDiff(lineCount: number, contentLength = 4): ParsedDiff {
  return makeDiff({ hunks: [makeHunk(lineCount, { contentLength })] });
}

describe("estimatePlaceholderMinHeight", () => {
  it("reserves a message body for binary and zero-hunk diffs", () => {
    const empty = estimatePlaceholderMinHeight(makeDiff(), true, 0);
    const binary = estimatePlaceholderMinHeight(
      makeDiff({ isBinary: true }),
      true,
      0,
    );
    // Both render a centered "No changes" / "Binary file not shown" body:
    // 44px header + 85px body. More than the bare header (the old undershoot bug).
    expect(empty).toBe(44 + 85);
    expect(binary).toBe(empty);
  });

  it("does not count header-type lines as body rows", () => {
    const withHeader = estimatePlaceholderMinHeight(
      makeDiff({ hunks: [makeHunk(3, { withHeaderLine: true })] }),
      true,
      3,
    );
    const withoutHeader = estimatePlaceholderMinHeight(
      makeDiff({ hunks: [makeHunk(3)] }),
      true,
      3,
    );
    expect(withHeader).toBe(withoutHeader);
  });

  it("grows by one row height per additional diff line", () => {
    const five = estimatePlaceholderMinHeight(wholeFileDiff(5), true, 5);
    const six = estimatePlaceholderMinHeight(wholeFileDiff(6), true, 6);
    expect(six - five).toBe(20); // one logical row: 16px line + 4px padding
  });

  it("reserves an expand-up row when the first hunk starts after line 1", () => {
    // base: hunk at lines 1-3, reaching EOF -> no expand rows.
    const base = estimatePlaceholderMinHeight(
      makeDiff({ hunks: [makeHunk(3, { newStart: 1, newCount: 3 })] }),
      true,
      3,
    );
    // shifted: hunk at lines 5-7, reaching EOF -> only an expand-up row differs.
    const withExpandUp = estimatePlaceholderMinHeight(
      makeDiff({ hunks: [makeHunk(3, { newStart: 5, newCount: 3 })] }),
      true,
      7,
    );
    expect(withExpandUp - base).toBe(20); // one expand row
  });

  it("reserves two expand rows for an inter-hunk gap", () => {
    // adjacent hunks (1-3, 4-6): no gap, reaching EOF -> no expand rows.
    const adjacent = estimatePlaceholderMinHeight(
      makeDiff({
        hunks: [
          makeHunk(3, { newStart: 1, newCount: 3 }),
          makeHunk(3, { newStart: 4, newCount: 3 }),
        ],
      }),
      true,
      6,
    );
    // gapped hunks (1-3, 10-12): a gap -> expand-down for the first + expand-up
    // for the second = two rows; reaches EOF so no trailing expand-down.
    const gapped = estimatePlaceholderMinHeight(
      makeDiff({
        hunks: [
          makeHunk(3, { newStart: 1, newCount: 3 }),
          makeHunk(3, { newStart: 10, newCount: 3 }),
        ],
      }),
      true,
      12,
    );
    expect(gapped - adjacent).toBe(2 * 20); // two expand rows
  });

  it("reserves an expand-down row only when the last hunk stops before EOF", () => {
    const atEof = estimatePlaceholderMinHeight(
      makeDiff({ hunks: [makeHunk(3, { newStart: 1, newCount: 3 })] }),
      true,
      3, // last line 3 == totalLines -> no expand-down
    );
    const beforeEof = estimatePlaceholderMinHeight(
      makeDiff({ hunks: [makeHunk(3, { newStart: 1, newCount: 3 })] }),
      true,
      100, // last line 3 < totalLines -> expand-down
    );
    expect(beforeEof - atEof).toBe(20); // one expand row
  });

  it("reserves more height for long lines only when wrapping is on", () => {
    // 120 chars wraps to 2 visual rows at WRAP_COLS=80; 40 chars stays 1 row.
    const oneRow = wholeFileDiff(1, 40);
    const twoRows = wholeFileDiff(1, 120);

    // Wrapping on: the extra visual line adds one line-height (16px), not a full
    // 20px row — padding applies once per logical row.
    expect(
      estimatePlaceholderMinHeight(twoRows, true, 1) -
        estimatePlaceholderMinHeight(oneRow, true, 1),
    ).toBe(16);

    // Wrapping off: long lines scroll horizontally, so both occupy one row.
    expect(estimatePlaceholderMinHeight(twoRows, false, 1)).toBe(
      estimatePlaceholderMinHeight(oneRow, false, 1),
    );
  });
});

describe("FilePlaceholder", () => {
  it("applies the estimated min-height as an inline style", () => {
    const diff = wholeFileDiff(5);
    const { container } = render(
      <FilePlaceholder diff={diff} fileName="a.txt" wrapLines totalLines={5} />,
    );
    const el = container.firstElementChild as HTMLElement;
    expect(el.style.minHeight).toBe(
      `${estimatePlaceholderMinHeight(diff, true, 5)}px`,
    );
  });
});
