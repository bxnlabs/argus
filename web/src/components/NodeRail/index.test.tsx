import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";

afterEach(() => { cleanup(); });
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { NodeContext } from "@/contexts/NodeContext";
import { TooltipProvider } from "@/components/ui/tooltip";
import { NodeRail } from "./index";
import type { NodeWithStatus } from "@/types";

function renderRail(nodes: NodeWithStatus[], activeId: string) {
  const active = nodes.find((n) => n.id === activeId) ?? null;
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <TooltipProvider>
        <NodeContext.Provider
          value={{ nodes, isLoaded: true, activeNodeId: activeId, activeNode: active, setActiveNode: vi.fn() }}
        >
          <NodeRail />
        </NodeContext.Provider>
      </TooltipProvider>
    </QueryClientProvider>,
  );
}

const base: NodeWithStatus = { id: "x", name: "x", url: "", source: "manual", self: false, summary: null, online: true, pending: false };

describe("NodeRail", () => {
  it("shows the current node's tile even when it's the only node", () => {
    const { container } = renderRail([{ ...base, id: "local", self: true }], "local");
    // The rail is present so the manage entry stays reachable...
    expect(container.querySelector("[data-testid='node-rail']")).not.toBeNull();
    expect(screen.getByLabelText("Add node")).toBeTruthy();
    // ...and the current node always shows, so you can see which node you're on
    // even before any peer is discovered (parity with the mobile switcher).
    expect(screen.getByTestId("node-tile-local")).toBeTruthy();
  });

  it("shows an attention badge with the count", () => {
    renderRail(
      [
        { ...base, id: "local", self: true },
        { ...base, id: "m1", name: "gpu", summary: { attention: 4, busy: 1, total: 6 } },
      ],
      "local",
    );
    expect(screen.getByTestId("node-attention-m1").textContent).toBe("4");
  });

  it("shows the attention badge on the selected node too", () => {
    // Sessions on the active node can still need attention while you're looking
    // at a different session, so its badge stays visible.
    renderRail(
      [
        { ...base, id: "local", self: true, summary: { attention: 2, busy: 0, total: 3 } },
        { ...base, id: "m1", name: "gpu" },
      ],
      "local",
    );
    expect(screen.getByTestId("node-attention-local").textContent).toBe("2");
  });

  it("marks an offline node", () => {
    renderRail(
      [
        { ...base, id: "local", self: true },
        { ...base, id: "m1", name: "gpu", online: false },
      ],
      "local",
    );
    expect(screen.getByTestId("node-tile-m1").getAttribute("data-online")).toBe("false");
  });

  it("offsets the working ring past the tile border on a busy peer", () => {
    renderRail(
      [
        { ...base, id: "local", self: true },
        { ...base, id: "m1", name: "gpu", summary: { attention: 0, busy: 2, total: 4 } },
      ],
      "local",
    );
    const tile = screen.getByTestId("node-tile-m1");
    // A busy non-active peer carries the spinning working ring...
    expect(tile.className).toContain("node-working");
    // ...and exposes its 3px border so the ring offsets past it instead of
    // landing on the border band (desktop/mobile parity — the borderless mobile
    // avatar leaves the var at its 0 default). See .node-working::before.
    expect(tile.style.getPropertyValue("--node-working-border")).toBe("3px");
  });

  it("shows the dog-ear cue only on Custom (manual) tiles", () => {
    renderRail(
      [
        { ...base, id: "local", self: true, source: "local" },
        { ...base, id: "m1", name: "gpu", source: "manual" },
        { ...base, id: "d1", name: "disco", source: "discovered" },
      ],
      "local",
    );
    // Manual node carries the affordance; local + discovered do not.
    expect(screen.getByTestId("node-dogear-m1")).toBeTruthy();
    expect(screen.queryByTestId("node-dogear-local")).toBeNull();
    expect(screen.queryByTestId("node-dogear-d1")).toBeNull();
  });
});
