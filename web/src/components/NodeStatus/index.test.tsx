import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, cleanup, fireEvent } from "@testing-library/react";

afterEach(() => { cleanup(); });
import { NodeContext } from "@/contexts/NodeContext";
import { TooltipProvider } from "@/components/ui/tooltip";
import { NodeStatus } from "./index";
import type { NodeWithStatus } from "@/types";

const base: NodeWithStatus = {
  id: "local", name: "my-laptop", url: "", source: "local",
  self: true, summary: null, online: true, pending: false,
};

function renderStatus(
  active: NodeWithStatus | null,
  onToggleRail = vi.fn(),
  railOpen = false,
) {
  render(
    <TooltipProvider>
      <NodeContext.Provider
        value={{
          nodes: active ? [active] : [],
          isLoaded: true,
          activeNodeId: active?.id ?? null,
          activeNode: active,
          setActiveNode: vi.fn(),
        }}
      >
        <NodeStatus railOpen={railOpen} onToggleRail={onToggleRail} />
      </NodeContext.Provider>
    </TooltipProvider>,
  );
  return onToggleRail;
}

describe("NodeStatus", () => {
  it("renders the active node name and an Online status", () => {
    renderStatus(base);
    expect(screen.getByText("my-laptop")).toBeTruthy();
    expect(screen.getByText("Online")).toBeTruthy();
  });

  it("renders Offline for a settled, unreachable node", () => {
    renderStatus({ ...base, online: false, pending: false });
    expect(screen.getByText("Offline")).toBeTruthy();
  });

  it("renders Connecting… while the first poll is in flight", () => {
    renderStatus({ ...base, online: false, pending: true });
    expect(screen.getByText("Connecting…")).toBeTruthy();
  });

  it("calls onToggleRail when clicked", () => {
    const onToggleRail = renderStatus(base);
    fireEvent.click(screen.getByTestId("node-status"));
    expect(onToggleRail).toHaveBeenCalledTimes(1);
  });

  it("falls back to a Manage nodes toggle when there is no active node", () => {
    const onToggleRail = vi.fn();
    render(
      <TooltipProvider>
        <NodeContext.Provider
          value={{ nodes: [], isLoaded: true, activeNodeId: null, activeNode: null, setActiveNode: vi.fn() }}
        >
          <NodeStatus railOpen={false} onToggleRail={onToggleRail} />
        </NodeContext.Provider>
      </TooltipProvider>,
    );
    // The rail stays reachable: the snippet renders a toggle even with no node,
    // so the add-node entry point inside the rail isn't orphaned.
    expect(screen.getByText("Manage nodes")).toBeTruthy();
    fireEvent.click(screen.getByTestId("node-status"));
    expect(onToggleRail).toHaveBeenCalledTimes(1);
  });

  it("button has an accessible label that includes the node name, status, and action", () => {
    renderStatus(base);
    expect(screen.getByRole("button", { name: /toggle node rail/i })).toBeTruthy();
  });

  it("reflects the rail's open/closed state via aria-expanded and aria-controls", () => {
    renderStatus(base, vi.fn(), false);
    const closed = screen.getByTestId("node-status");
    expect(closed.getAttribute("aria-expanded")).toBe("false");
    // Rail is unmounted while closed, so don't dangle an IDREF at an absent element.
    expect(closed.getAttribute("aria-controls")).toBeNull();
    cleanup();
    renderStatus(base, vi.fn(), true);
    const open = screen.getByTestId("node-status");
    expect(open.getAttribute("aria-expanded")).toBe("true");
    expect(open.getAttribute("aria-controls")).toBe("node-rail");
  });
});
