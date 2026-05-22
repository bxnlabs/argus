import { describe, it, expect } from "vitest";
import { locateSnippetAnchor } from "./UnanchoredCommentSection";

describe("locateSnippetAnchor", () => {
  it("returns the index of a uniquely-occurring snippet", () => {
    const lines = ["function foo() {", "  return 1;", "}"];
    expect(locateSnippetAnchor(lines, "  return 1;")).toBe(1);
  });

  it("locates the first line and the last line", () => {
    const lines = ["a", "b", "c"];
    expect(locateSnippetAnchor(lines, "a")).toBe(0);
    expect(locateSnippetAnchor(lines, "c")).toBe(2);
  });

  it("returns -1 when the snippet is repeated (ambiguous)", () => {
    const lines = ["if (x) {", "  doThing();", "}", "if (y) {", "  doOther();", "}"];
    // "}" appears twice — we can't tell which line is the anchor.
    expect(locateSnippetAnchor(lines, "}")).toBe(-1);
  });

  it("returns -1 when the snippet is absent", () => {
    expect(locateSnippetAnchor(["a", "b"], "c")).toBe(-1);
  });

  it("returns -1 for an empty snippet", () => {
    expect(locateSnippetAnchor(["a", "", "b"], "")).toBe(-1);
  });

  it("treats a single-line context as a unique match", () => {
    expect(locateSnippetAnchor(["const x = 1;"], "const x = 1;")).toBe(0);
  });

  it("matches a line with leading whitespace exactly", () => {
    const lines = ["foo", "    bar", "baz"];
    expect(locateSnippetAnchor(lines, "    bar")).toBe(1);
    expect(locateSnippetAnchor(lines, "bar")).toBe(-1);
  });
});
