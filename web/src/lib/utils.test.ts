import { describe, it, expect } from "vitest";
import { contractTilde, compressPath } from "./utils";

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

  it("second stage drops first segment when still over threshold", () => {
    expect(
      compressPath("/opt/data/very/deep/nested/project", home, 20),
    ).toBe("/.../nested/project");
  });

  it("second stage with tilde prefix", () => {
    expect(
      compressPath(
        "/Users/jeevb/Workspace/repos/bxnlabs/very-long-project-name",
        home,
        20,
      ),
    ).toBe("~/.../bxnlabs/very-long-project-name");
  });

  it("does not compress 3-segment path even over threshold", () => {
    expect(compressPath("/Users/jeevb/project", home, 10)).toBe("~/project");
  });

  it("handles exactly at threshold without compressing", () => {
    expect(compressPath("/Users/jeevb/short", home, 7)).toBe("~/short");
  });

  it("compresses with empty home", () => {
    expect(
      compressPath("/Users/jeevb/Workspace/repos/bxnlabs/argus", "", 30),
    ).toBe("/Users/.../bxnlabs/argus");
  });
});
