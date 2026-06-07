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

const base: NodeWithStatus = { id: "x", name: "x", url: "", source: "manual", self: false, summary: null, online: true };

describe("NodeRail", () => {
  it("renders nothing with a single node", () => {
    const { container } = renderRail([{ ...base, id: "local", self: true }], "local");
    expect(container.querySelector("[data-testid='node-rail']")).toBeNull();
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
});
