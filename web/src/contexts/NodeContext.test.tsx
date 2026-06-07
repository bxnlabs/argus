import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, act, waitFor, cleanup } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { NodeProvider, useNodeContext } from "./NodeContext";
import { getNodeBaseUrl, setActiveNodeBaseUrl } from "@/api/client";

function Probe() {
  const { nodes, activeNode, setActiveNode } = useNodeContext();
  return (
    <div>
      <span data-testid="active">{activeNode?.id ?? "none"}</span>
      <button onClick={() => setActiveNode("m1")}>switch</button>
      <span data-testid="count">{nodes.length}</span>
    </div>
  );
}

beforeEach(() => {
  localStorage.clear();
  vi.stubGlobal("fetch", vi.fn(async (input: string) => {
    if (input === "/api/nodes") {
      return new Response(JSON.stringify({ nodes: [
        { id: "local", name: "this", url: "", source: "local", self: true },
        { id: "m1", name: "gpu", url: "http://gpu:80", source: "manual", self: false },
      ] }), { status: 200 });
    }
    return new Response(JSON.stringify({ attention: 0, busy: 0, total: 0 }), { status: 200 });
  }));
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  setActiveNodeBaseUrl("");
});

describe("NodeProvider", () => {
  it("defaults to the local node and switches base URL on select", async () => {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(
      <QueryClientProvider client={qc}>
        <NodeProvider><Probe /></NodeProvider>
      </QueryClientProvider>,
    );

    await waitFor(() => expect(screen.getByTestId("count").textContent).toBe("2"));
    expect(screen.getByTestId("active").textContent).toBe("local");
    expect(getNodeBaseUrl()).toBe("");

    await act(async () => { screen.getByText("switch").click(); });
    expect(screen.getByTestId("active").textContent).toBe("m1");
    expect(getNodeBaseUrl()).toBe("http://gpu:80");
    expect(localStorage.getItem("argus.activeNodeId")).toBe("m1");
  });

  it("evicts node-scoped caches when a persisted remote node resolves on load", async () => {
    // A remote node was active in a prior session.
    localStorage.setItem("argus.activeNodeId", "m1");
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    // Seed a node-scoped cache (the local node's sessions, fetched same-origin
    // during the brief pre-registry window) and a rail cache that must survive
    // the implicit switch to the persisted remote node.
    qc.setQueryData(["sessions", "list"], ["stale-local-session"]);
    qc.setQueryData(["node-summary", "keep"], { attention: 9, busy: 0, total: 9 });

    render(
      <QueryClientProvider client={qc}>
        <NodeProvider><Probe /></NodeProvider>
      </QueryClientProvider>,
    );

    // Once the registry loads, "m1" resolves as active and its origin is
    // applied — without any user click.
    await waitFor(() => expect(getNodeBaseUrl()).toBe("http://gpu:80"));
    expect(screen.getByTestId("active").textContent).toBe("m1");

    // The local node's stale sessions are dropped so they never flash into the
    // remote node; the rail's registry/summary caches are preserved.
    expect(qc.getQueryData(["sessions", "list"])).toBeUndefined();
    expect(qc.getQueryData(["node-summary", "keep"])).toEqual({ attention: 9, busy: 0, total: 9 });
  });
});
