import { describe, it, expect } from "vitest";
import { render } from "@testing-library/react";
import { FilePlaceholder, FILE_HEADER_HEIGHT_PX, PLACEHOLDER_LINE_HEIGHT_PX } from "./LazyFileDiff";
import type { ParsedDiff } from "@/lib/diff-parser";

function makeDiff(overrides: Partial<ParsedDiff> = {}): ParsedDiff {
  return {
    oldFile: "a.txt",
    newFile: "a.txt",
    isBinary: false,
    hunks: [],
    additions: 0,
    deletions: 0,
    isNew: false,
    isDeleted: false,
    isRenamed: false,
    ...overrides,
  };
}

describe("FilePlaceholder", () => {
  it("reserves header height when totalLines is 0", () => {
    const { container } = render(
      <FilePlaceholder diff={makeDiff()} fileName="a.txt" totalLines={0} />,
    );
    const el = container.firstElementChild as HTMLElement;
    expect(el.style.minHeight).toBe(`${FILE_HEADER_HEIGHT_PX}px`);
  });

  it("adds per-line height for each line in totalLines", () => {
    const { container } = render(
      <FilePlaceholder diff={makeDiff()} fileName="a.txt" totalLines={100} />,
    );
    const el = container.firstElementChild as HTMLElement;
    const expected = FILE_HEADER_HEIGHT_PX + 100 * PLACEHOLDER_LINE_HEIGHT_PX;
    expect(el.style.minHeight).toBe(`${expected}px`);
  });

  it("uses the published constants (44px header, 20px per line)", () => {
    expect(FILE_HEADER_HEIGHT_PX).toBe(44);
    expect(PLACEHOLDER_LINE_HEIGHT_PX).toBe(20);
  });
});
