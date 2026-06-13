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
  onDismiss = vi.fn(),
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
        <MobileNodePanel open onClose={onClose} onDismiss={onDismiss} />
      </NodeContext.Provider>
    </QueryClientProvider>,
  );
  return { setActiveNode, onClose, onDismiss };
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
    // Stays open so you can switch freely; exits only via the back chevron
    // (onClose) or the dimmed area / Escape (onDismiss).
    expect(onClose).not.toHaveBeenCalled();
  });

  it("steps back one level (onClose, not onDismiss) when the back chevron is tapped", () => {
    const { onClose, onDismiss } = renderPanel();
    fireEvent.click(screen.getByLabelText("Back to sidebar"));
    // Back returns to the sidebar — closes the rail, leaves the drawer open.
    expect(onClose).toHaveBeenCalledTimes(1);
    expect(onDismiss).not.toHaveBeenCalled();
  });

  it("dismisses the whole stack (onDismiss, not onClose) on Escape / tap-outside", () => {
    const { onClose, onDismiss } = renderPanel();
    // Escape and the dimmed-overlay tap share Radix's onOpenChange path; Escape
    // is the testable proxy. Tapping outside should land back on the main panel,
    // so it closes the drawer too — never the back-one-level onClose.
    fireEvent.keyDown(document.body, { key: "Escape" });
    expect(onDismiss).toHaveBeenCalledTimes(1);
    expect(onClose).not.toHaveBeenCalled();
  });

  it("offers an Add node entry point", () => {
    renderPanel();
    expect(screen.getByText("Add node")).toBeTruthy();
  });

  it("shows the working ring on a busy peer, matching the desktop rail", () => {
    const busy: NodeWithStatus = {
      id: "m2", name: "gpu", url: "http://gpu", source: "discovered",
      self: false, summary: { attention: 0, busy: 2, total: 2 }, online: true, pending: false,
    };
    render(
      <QueryClientProvider client={new QueryClient()}>
        <NodeContext.Provider
          value={{
            nodes: [local, busy], isLoaded: true, activeNodeId: "local",
            activeNode: local, setActiveNode: vi.fn(),
          }}
        >
          <MobileNodePanel open onClose={vi.fn()} onDismiss={vi.fn()} />
        </NodeContext.Provider>
      </QueryClientProvider>,
    );
    // The busy, non-active, online peer carries the spinning ring...
    expect(screen.getByTestId("node-row-m2").querySelector(".node-working")).not.toBeNull();
    // ...while the active node (you're already looking at it) never does.
    expect(screen.getByTestId("node-row-local").querySelector(".node-working")).toBeNull();
  });

  it("offers rename/delete actions only for Custom (manual) nodes", () => {
    renderPanel();
    // The Tailscale-discovered node (m1) is not editable...
    expect(screen.queryByLabelText("Actions for argus-bumblebee")).toBeNull();
    // ...so neither is the local node. (Both rows render, neither has a menu.)
    expect(screen.queryByLabelText("Actions for prime")).toBeNull();
  });
});
