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
    // White is the shared currency color across all three rails (session list,
    // node rail, view-mode rail), which leaves blue to mean "unread" alone.
    const { container } = renderRail("git");
    const pills = container.querySelectorAll("[data-testid='view-mode-pill']");
    expect(pills.length).toBe(1);
    expect(pills[0].className).toContain("bg-white");
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
