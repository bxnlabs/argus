import { describe, it, expect } from "vitest";
import { parseDiff } from "./diff-parser";

describe("parseDiff hunk keys", () => {
  const simpleDiff = `--- a/file.txt
+++ b/file.txt
@@ -1,3 +1,4 @@
 line1
+added
 line2
 line3
@@ -10,3 +11,3 @@
 line10
-old
+new
 line12`;

  it("produces hunks with distinct oldStart-newStart pairs", () => {
    const result = parseDiff(simpleDiff);
    const keys = result.hunks.map((h) => `${h.oldStart}:${h.newStart}`);
    expect(new Set(keys).size).toBe(keys.length);
  });

  it("lines within a hunk have stable identifiers from type + line numbers", () => {
    const result = parseDiff(simpleDiff);
    const firstHunk = result.hunks[0];
    // Each line should have a unique combination of type + oldLineNumber + newLineNumber
    const lineKeys = firstHunk.lines.map(
      (l) => `${l.type}:${l.oldLineNumber ?? ""}:${l.newLineNumber ?? ""}`,
    );
    expect(new Set(lineKeys).size).toBe(lineKeys.length);
  });
});
