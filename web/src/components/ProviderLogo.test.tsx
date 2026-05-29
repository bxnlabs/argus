import { describe, it, expect } from "vitest";
import { render } from "@testing-library/react";
import { ProviderLogo } from "./ProviderLogo";

describe("ProviderLogo", () => {
  it("renders a labeled brand <svg> for a known provider", () => {
    const { container } = render(<ProviderLogo type="claude" />);
    const svg = container.querySelector("svg");
    expect(svg).not.toBeNull();
    expect(svg?.getAttribute("aria-label")).toBe("Claude");
  });

  it("falls back to a terminal glyph for shell", () => {
    const { container } = render(<ProviderLogo type="shell" />);
    const svg = container.querySelector("svg");
    expect(svg).not.toBeNull();
    expect(svg?.getAttribute("class") ?? "").toContain("lucide-terminal");
    expect(svg?.getAttribute("aria-label")).toBe("Terminal");
  });

  it("applies the provided className for sizing", () => {
    const { container } = render(
      <ProviderLogo type="gemini" className="h-4 w-4" />,
    );
    expect(container.querySelector("svg")?.getAttribute("class") ?? "").toContain(
      "h-4",
    );
  });
});
