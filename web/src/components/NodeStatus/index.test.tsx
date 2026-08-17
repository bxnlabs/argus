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
  it("renders the active node's avatar tile with an online status dot", () => {
    renderStatus(base);
    expect(screen.getByTestId("node-avatar-dot").className).toContain("bg-green-500");
    // The tile is just the avatar now; name + status ride on the label/tooltip.
    const label = screen.getByTestId("node-status").getAttribute("aria-label") ?? "";
    expect(label).toContain("my-laptop");
    expect(label).toContain("Online");
  });

  it("reads Offline for a settled, unreachable node", () => {
    renderStatus({ ...base, online: false, pending: false });
    expect(screen.getByTestId("node-avatar-dot").className).toContain("bg-muted-foreground");
    expect(screen.getByTestId("node-status").getAttribute("aria-label")).toContain("Offline");
  });

  it("reads Connecting… while the first poll is in flight", () => {
    renderStatus({ ...base, online: false, pending: true });
    expect(screen.getByTestId("node-avatar-dot").className).toContain("bg-amber-500");
    expect(screen.getByTestId("node-status").getAttribute("aria-label")).toContain("Connecting…");
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
    expect(screen.getByTestId("node-status").getAttribute("aria-label")).toContain("Add node");
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

  function renderWithNodes(nodes: NodeWithStatus[], activeId: string, railOpen: boolean) {
    render(
      <TooltipProvider>
        <NodeContext.Provider
          value={{
            nodes,
            isLoaded: true,
            activeNodeId: activeId,
            activeNode: nodes.find((n) => n.id === activeId) ?? null,
            setActiveNode: vi.fn(),
          }}
        >
          <NodeStatus railOpen={railOpen} onToggleRail={vi.fn()} />
        </NodeContext.Provider>
      </TooltipProvider>,
    );
  }

  const peer = (over: Partial<NodeWithStatus>): NodeWithStatus => ({
    ...base, self: false, source: "manual", ...over,
  });

  it("rings the bell when other online nodes have unread, without printing a count", () => {
    renderWithNodes(
      [
        { ...base, summary: { attention: 1, busy: 0, total: 1 } }, // active — excluded
        peer({ id: "m1", summary: { attention: 2, busy: 0, total: 2 } }),
        peer({ id: "m2", summary: { attention: 3, busy: 0, total: 3 } }),
      ],
      "local",
      false,
    );
    const bell = screen.getByTestId("node-status-attention");
    // A digit on the current node's avatar would read as that node's own count.
    expect(bell.textContent).toBe("");
  });

  it("names the waiting count in the bell's accessible label", () => {
    renderWithNodes(
      [
        { ...base, summary: { attention: 1, busy: 0, total: 1 } }, // active — excluded
        peer({ id: "m1", summary: { attention: 2, busy: 0, total: 2 } }),
        peer({ id: "m2", summary: { attention: 3, busy: 0, total: 3 } }),
      ],
      "local",
      false,
    );
    // 2 + 3 from the peers; the active node's own 1 is not counted.
    const label = screen.getByTestId("node-status").getAttribute("aria-label") ?? "";
    expect(label).toContain("5 unread on other nodes");
  });

  it("says one node in the singular", () => {
    renderWithNodes(
      [{ ...base }, peer({ id: "m1", summary: { attention: 1, busy: 0, total: 1 } })],
      "local",
      false,
    );
    const label = screen.getByTestId("node-status").getAttribute("aria-label") ?? "";
    expect(label).toContain("1 unread on another node");
  });

  it("leaves the label free of unread wording when nothing is waiting", () => {
    renderWithNodes([{ ...base }, peer({ id: "m1" })], "local", false);
    expect(screen.getByTestId("node-status").getAttribute("aria-label")).not.toContain("unread");
  });

  it("omits offline nodes from the aggregate and hides the badge at zero", () => {
    renderWithNodes(
      [
        { ...base },
        peer({ id: "m1", online: false, summary: { attention: 4, busy: 0, total: 4 } }),
      ],
      "local",
      false,
    );
    expect(screen.queryByTestId("node-status-attention")).toBeNull();
  });

  it("drops the aggregate badge once the rail is open (counts move to the tiles)", () => {
    renderWithNodes(
      [
        { ...base },
        peer({ id: "m1", summary: { attention: 2, busy: 0, total: 2 } }),
      ],
      "local",
      true,
    );
    expect(screen.queryByTestId("node-status-attention")).toBeNull();
  });

  it("reflects the open state on the card via data-state", () => {
    renderStatus(base, vi.fn(), false);
    expect(screen.getByTestId("node-status").getAttribute("data-state")).toBe("closed");
    cleanup();
    renderStatus(base, vi.fn(), true);
    expect(screen.getByTestId("node-status").getAttribute("data-state")).toBe("open");
  });
});
