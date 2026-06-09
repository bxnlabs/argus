import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, cleanup, fireEvent } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

afterEach(() => { cleanup(); });
import { NodeContext } from "@/contexts/NodeContext";
import { MobileNodePanel } from "./MobileNodePanel";
import type { NodeWithStatus } from "@/types";

const local: NodeWithStatus = {
  id: "local", name: "prime", url: "", source: "local",
  self: true, summary: null, online: true, pending: false,
};
const remote: NodeWithStatus = {
  id: "m1", name: "argus-bumblebee", url: "http://bee", source: "discovered",
  self: false, summary: null, online: false, pending: false,
};

function renderPanel(
  setActiveNode = vi.fn(),
  onClose = vi.fn(),
  activeNodeId = "local",
) {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <NodeContext.Provider
        value={{
          nodes: [local, remote],
          isLoaded: true,
          activeNodeId,
          activeNode: [local, remote].find((n) => n.id === activeNodeId) ?? null,
          setActiveNode,
        }}
      >
        <MobileNodePanel open onClose={onClose} />
      </NodeContext.Provider>
    </QueryClientProvider>,
  );
  return { setActiveNode, onClose };
}

describe("MobileNodePanel", () => {
  it("renders a row per node with name and 'source · status' subtitle", () => {
    renderPanel();
    expect(screen.getByText("prime")).toBeTruthy();
    expect(screen.getByText("argus-bumblebee")).toBeTruthy();
    // Subtitle pairs the node's origin with its connection status.
    expect(screen.getByText("This machine · Online")).toBeTruthy();
    expect(screen.getByText("Tailscale · Offline")).toBeTruthy();
  });

  it("marks the active node row with aria-current", () => {
    renderPanel(vi.fn(), vi.fn(), "local");
    expect(screen.getByTestId("node-row-local").getAttribute("aria-current")).toBe("true");
    expect(screen.getByTestId("node-row-m1").getAttribute("aria-current")).toBe("false");
  });

  it("switches to a node without closing the panel when its row is tapped", () => {
    const { setActiveNode, onClose } = renderPanel();
    fireEvent.click(screen.getByTestId("node-row-m1"));
    expect(setActiveNode).toHaveBeenCalledWith("m1");
    // Stays open so you can switch freely; closed only via back / dimmed area.
    expect(onClose).not.toHaveBeenCalled();
  });

  it("offers a Manage nodes entry point", () => {
    renderPanel();
    expect(screen.getByText("Manage nodes…")).toBeTruthy();
  });
});
