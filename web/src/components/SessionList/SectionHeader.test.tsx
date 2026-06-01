import { describe, it, expect, vi, afterEach } from "vitest";
import { render, fireEvent, cleanup } from "@testing-library/react";
import { SectionHeader } from "./index";

afterEach(cleanup);

describe("SectionHeader", () => {
  it("renders the label, marks itself expanded, and rotates the chevron when not collapsed", () => {
    const { getByRole, container } = render(
      <SectionHeader collapsed={false} onToggle={() => {}}>
        Pinned
      </SectionHeader>,
    );
    const button = getByRole("button");
    expect(button.getAttribute("aria-expanded")).toBe("true");
    expect(button.textContent).toContain("Pinned");
    const chevron = container.querySelector("svg");
    expect(chevron?.getAttribute("class") ?? "").toContain("rotate-90");
  });

  it("marks itself collapsed and un-rotates the chevron when collapsed", () => {
    const { getByRole, container } = render(
      <SectionHeader collapsed={true} onToggle={() => {}}>
        Recents
      </SectionHeader>,
    );
    const button = getByRole("button");
    expect(button.getAttribute("aria-expanded")).toBe("false");
    const chevron = container.querySelector("svg");
    expect(chevron?.getAttribute("class") ?? "").not.toContain("rotate-90");
  });

  it("invokes onToggle when clicked", () => {
    const onToggle = vi.fn();
    const { getByRole } = render(
      <SectionHeader collapsed={false} onToggle={onToggle}>
        Pinned
      </SectionHeader>,
    );
    fireEvent.click(getByRole("button"));
    expect(onToggle).toHaveBeenCalledTimes(1);
  });
});
