import { describe, it, expect, vi, afterEach } from "vitest";
import { copyToClipboard } from "./clipboard";

afterEach(() => {
  vi.restoreAllMocks();
});

describe("copyToClipboard", () => {
  it("uses the async clipboard API when available", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", {
      value: { writeText },
      configurable: true,
    });
    const ok = await copyToClipboard("hello");
    expect(ok).toBe(true);
    expect(writeText).toHaveBeenCalledWith("hello");
  });

  it("falls back to execCommand when the clipboard API is unavailable", async () => {
    Object.defineProperty(navigator, "clipboard", {
      value: undefined,
      configurable: true,
    });
    if (typeof document.execCommand !== "function") {
      (document as unknown as { execCommand: () => boolean }).execCommand =
        () => false;
    }
    const exec = vi.spyOn(document, "execCommand").mockReturnValue(true);
    const ok = await copyToClipboard("hi");
    expect(ok).toBe(true);
    expect(exec).toHaveBeenCalledWith("copy");
  });
});
