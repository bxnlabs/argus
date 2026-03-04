import { describe, it, expect } from "vitest";
import { contractTilde, compressPath, truncateRight } from "./utils";

describe("contractTilde", () => {
  const home = "/Users/jeevb";

  it("replaces home prefix with ~", () => {
    expect(contractTilde("/Users/jeevb/project", home)).toBe("~/project");
  });

  it("returns ~ for exact home path", () => {
    expect(contractTilde("/Users/jeevb", home)).toBe("~");
  });

  it("leaves non-home paths unchanged", () => {
    expect(contractTilde("/opt/data", home)).toBe("/opt/data");
  });

  it("leaves path unchanged when homePath is empty", () => {
    expect(contractTilde("/Users/jeevb/project", "")).toBe(
      "/Users/jeevb/project",
    );
  });
});

describe("truncateRight", () => {
  it("returns string unchanged when within limit", () => {
    expect(truncateRight("main", 20)).toBe("main");
  });

  it("returns string unchanged at exact limit", () => {
    expect(truncateRight("abcde", 5)).toBe("abcde");
  });

  it("truncates with ellipsis when over limit", () => {
    expect(truncateRight("abcdefghij", 5)).toBe("abcd…");
  });

  it("returns ellipsis for max 1", () => {
    expect(truncateRight("abcde", 1)).toBe("…");
  });

  it("returns empty for max 0", () => {
    expect(truncateRight("abcde", 0)).toBe("");
  });

  it("returns empty for negative max", () => {
    expect(truncateRight("abcde", -1)).toBe("");
  });

  it("returns empty string unchanged", () => {
    expect(truncateRight("", 10)).toBe("");
  });
});

describe("compressPath", () => {
  const home = "/Users/jeevb";

  it("returns short path unchanged", () => {
    expect(compressPath("/tmp/project", home, 40)).toBe("/tmp/project");
  });

  it("tilde-shortens home prefix", () => {
    expect(compressPath("/Users/jeevb/project", home, 40)).toBe("~/project");
  });

  it("returns ~ for home dir itself", () => {
    expect(compressPath("/Users/jeevb", home, 40)).toBe("~");
  });

  it("compresses long path keeping first + last 2 segments", () => {
    expect(
      compressPath("/Users/jeevb/Workspace/repos/bxnlabs/argus", home, 30),
    ).toBe("~/Workspace/.../bxnlabs/argus");
  });

  it("compresses non-home long path", () => {
    expect(
      compressPath("/opt/data/very/deep/nested/project", home, 25),
    ).toBe("/opt/.../nested/project");
  });

  it("drops first segment when still over threshold", () => {
    expect(
      compressPath("/opt/data/very/deep/nested/project", home, 20),
    ).toBe("/.../nested/project");
  });

  it("drops parent to preserve basename", () => {
    expect(
      compressPath(
        "/Users/jeevb/Workspace/repos/bxnlabs/very-long-project-name",
        home,
        20,
      ),
    ).toBe("~/.../very-long-pro…");
  });

  it("three-segment drops middle to preserve basename", () => {
    expect(
      compressPath("/Users/jeevb/Workspace/long-parent/project", home, 20),
    ).toBe("~/.../project");
  });

  it("three-segment fallback when basename too long", () => {
    expect(
      compressPath("/Users/jeevb/a/b/very-long-basename", home, 15),
    ).toBe("~/a/b/very-lon…");
  });

  it("truncates shallow path when over threshold", () => {
    expect(compressPath("/Users/jeevb/project", home, 10)).toBe("~/project");
  });

  it("handles exactly at threshold without compressing", () => {
    expect(compressPath("/Users/jeevb/short", home, 7)).toBe("~/short");
  });

  it("deep path with long tail preserves basename", () => {
    expect(
      compressPath(
        "/Users/jeevb/a/b/very-very-very-long-segment/another-very-very-long-segment",
        home,
        30,
      ),
    ).toBe("~/.../another-very-very-long-…");
  });

  it("shallow path truncated when over threshold", () => {
    expect(
      compressPath("/Users/jeevb/very-long-directory-name", home, 15),
    ).toBe("~/very-long-di…");
  });

  it("compresses with empty home", () => {
    expect(
      compressPath("/Users/jeevb/Workspace/repos/bxnlabs/argus", "", 30),
    ).toBe("/Users/.../bxnlabs/argus");
  });
});
