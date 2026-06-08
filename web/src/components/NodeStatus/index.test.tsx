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

function renderStatus(active: NodeWithStatus | null, onToggleRail = vi.fn()) {
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
        <NodeStatus onToggleRail={onToggleRail} />
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

  it("renders nothing when there is no active node", () => {
    const { container } = render(
      <TooltipProvider>
        <NodeContext.Provider
          value={{ nodes: [], isLoaded: true, activeNodeId: null, activeNode: null, setActiveNode: vi.fn() }}
        >
          <NodeStatus onToggleRail={vi.fn()} />
        </NodeContext.Provider>
      </TooltipProvider>,
    );
    expect(container.querySelector("[data-testid='node-status']")).toBeNull();
  });

  it("button has an accessible label that includes the node name, status, and action", () => {
    renderStatus(base);
    expect(screen.getByRole("button", { name: /toggle node rail/i })).toBeTruthy();
  });
});
