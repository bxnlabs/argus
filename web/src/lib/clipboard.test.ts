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

  it("falls back to execCommand when clipboard.writeText rejects", async () => {
    const writeText = vi.fn().mockRejectedValue(new Error("Permission denied"));
    Object.defineProperty(navigator, "clipboard", {
      value: { writeText },
      configurable: true,
    });
    const exec = vi.spyOn(document, "execCommand").mockReturnValue(true);
    const ok = await copyToClipboard("test");
    expect(ok).toBe(true);
    expect(exec).toHaveBeenCalledWith("copy");
  });

  it("creates the fallback textarea inside the given container so a focus trap can't steal the selection", async () => {
    Object.defineProperty(navigator, "clipboard", {
      value: undefined,
      configurable: true,
    });
    const container = document.createElement("div");
    document.body.appendChild(container);
    if (typeof document.execCommand !== "function") {
      (document as unknown as { execCommand: () => boolean }).execCommand =
        () => false;
    }
    // Capture where the textarea lives at the moment of copy.
    let textareaInContainer = false;
    vi.spyOn(document, "execCommand").mockImplementation(() => {
      textareaInContainer = container.querySelector("textarea") !== null;
      return true;
    });
    const ok = await copyToClipboard("scoped", container);
    expect(ok).toBe(true);
    expect(textareaInContainer).toBe(true);
    expect(container.querySelector("textarea")).toBeNull(); // cleaned up after
    container.remove();
  });
});
