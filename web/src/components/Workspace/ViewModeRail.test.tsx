import { describe, it, expect, vi, afterEach } from "vitest";
import { render, cleanup } from "@testing-library/react";
import { TooltipProvider } from "@/components/ui/tooltip";
import { ViewModeRail } from "./ViewModeRail";
import type { SidePanel } from "@/components/views/types";

afterEach(cleanup);

function renderRail(activePanel: SidePanel) {
  return render(
    <TooltipProvider>
      <ViewModeRail
        activePanel={activePanel}
        onSetActivePanel={vi.fn()}
        isGitEnabled
        isEditorEnabled
      />
    </TooltipProvider>,
  );
}

describe("ViewModeRail currency pill", () => {
  it("marks the active view with a white pill", () => {
    // White marks the current thing in all three rails, leaving blue for unread.
    const { container } = renderRail("git");
    const pills = container.querySelectorAll("[data-testid='view-mode-pill']");
    expect(pills.length).toBe(1);
    expect(pills[0].className).toContain("bg-white");
  });

  it("dims every mode but the active one", () => {
    // The pill says which mode is current; dimming keeps the others quiet.
    const { container } = renderRail("git");
    const dimmed = (label: string) =>
      container
        .querySelector(`[aria-label^='${label}']`)!
        .className.includes("text-muted-foreground");
    expect(dimmed("Git")).toBe(false);
    expect(dimmed("Terminal")).toBe(true);
    expect(dimmed("Editor")).toBe(true);
  });

  it("moves the pill when the active view changes", () => {
    const { container } = renderRail(null);
    const terminalPill = container
      .querySelector("[aria-label^='Terminal']")
      ?.querySelector("[data-testid='view-mode-pill']");
    expect(terminalPill).not.toBeNull();
    expect(
      container
        .querySelector("[aria-label^='Git']")
        ?.querySelector("[data-testid='view-mode-pill']"),
    ).toBeNull();
  });
});
