import { describe, it, expect } from "vitest";
import { getLanguageFromPath } from "./fileContent";

describe("getLanguageFromPath", () => {
  it("maps a known extension to its Monaco language id", () => {
    expect(getLanguageFromPath("/repo/a.ts")).toBe("typescript");
  });

  it("falls back for an unknown extension rather than throwing", () => {
    expect(typeof getLanguageFromPath("/repo/LICENSE")).toBe("string");
  });
});
