import { describe, it, expect } from "vitest";
import { insertPaths } from "./insertPaths";

describe("insertPaths", () => {
  it("inserts into empty text with no padding spaces", () => {
    expect(insertPaths("", 0, 0, ["/tmp/a.txt"])).toEqual({
      text: "/tmp/a.txt",
      cursor: 10,
    });
  });

  it("joins multiple paths with a single space", () => {
    expect(insertPaths("", 0, 0, ["/tmp/a", "/tmp/b"])).toEqual({
      text: "/tmp/a /tmp/b",
      cursor: 13,
    });
  });

  it("shell-escapes paths containing spaces", () => {
    const result = insertPaths("", 0, 0, ["/tmp/my file"]);
    expect(result.text).toBe("'/tmp/my file'");
    expect(result.cursor).toBe(14);
  });

  it("adds a leading space when the text before the cursor does not end in one", () => {
    expect(insertPaths("look at", 7, 7, ["/tmp/a"])).toEqual({
      text: "look at /tmp/a",
      cursor: 14,
    });
  });

  it("does not double the leading space when one is already there", () => {
    expect(insertPaths("look at ", 8, 8, ["/tmp/a"])).toEqual({
      text: "look at /tmp/a",
      cursor: 14,
    });
  });

  it("adds a trailing space when the text after the cursor does not start with one", () => {
    expect(insertPaths("ab", 1, 1, ["/tmp/a"])).toEqual({
      text: "a /tmp/a b",
      cursor: 8,
    });
  });

  it("replaces the current selection", () => {
    expect(insertPaths("keep DROP keep", 5, 9, ["/tmp/a"])).toEqual({
      text: "keep /tmp/a keep",
      cursor: 11,
    });
  });

  it("returns the text unchanged when given no paths", () => {
    expect(insertPaths("hello", 5, 5, [])).toEqual({
      text: "hello",
      cursor: 5,
    });
  });
});
