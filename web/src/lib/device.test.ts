import { describe, it, expect, vi, afterEach } from "vitest";

afterEach(() => {
  vi.unstubAllGlobals();
  vi.resetModules();
});

async function getIsMac() {
  const { isMac } = await import("./device");
  return isMac;
}

describe("isMac", () => {
  it("returns true for a macOS user agent", async () => {
    vi.stubGlobal("navigator", {
      userAgent:
        "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
    });
    const isMac = await getIsMac();
    expect(isMac()).toBe(true);
  });

  it("returns true for an iPhone user agent", async () => {
    vi.stubGlobal("navigator", {
      userAgent:
        "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15",
    });
    const isMac = await getIsMac();
    expect(isMac()).toBe(true);
  });

  it("returns false for a Windows user agent", async () => {
    vi.stubGlobal("navigator", {
      userAgent:
        "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
    });
    const isMac = await getIsMac();
    expect(isMac()).toBe(false);
  });

  it("returns false for a Linux user agent", async () => {
    vi.stubGlobal("navigator", {
      userAgent:
        "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
    });
    const isMac = await getIsMac();
    expect(isMac()).toBe(false);
  });

  it("returns false when navigator is undefined", async () => {
    vi.stubGlobal("navigator", undefined);
    const isMac = await getIsMac();
    expect(isMac()).toBe(false);
  });
});
