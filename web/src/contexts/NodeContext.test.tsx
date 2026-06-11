import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, act, waitFor, cleanup } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { NodeProvider, useNodeContext } from "./NodeContext";

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
});

describe("NodeProvider", () => {
  it("defaults to the local node and persists the selection on switch", async () => {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(
      <QueryClientProvider client={qc}>
        <NodeProvider><Probe /></NodeProvider>
      </QueryClientProvider>,
    );

    await waitFor(() => expect(screen.getByTestId("count").textContent).toBe("2"));
    expect(screen.getByTestId("active").textContent).toBe("local");

    await act(async () => { screen.getByText("switch").click(); });
    expect(screen.getByTestId("active").textContent).toBe("m1");
    expect(localStorage.getItem("argus.activeNodeId")).toBe("m1");
  });

  it("keeps each node's scoped caches across a switch (instant-switch-with-cache)", async () => {
    // A remote node was active in a prior session.
    localStorage.setItem("argus.activeNodeId", "m1");
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    // Seed a cache under the local node's scope ("local:"). With no eviction on
    // switch, it must survive the implicit switch to the persisted remote so
    // switching back later is instant.
    qc.setQueryData(["sessions", "local:", "list"], ["cached-local-session"]);

    render(
      <QueryClientProvider client={qc}>
        <NodeProvider><Probe /></NodeProvider>
      </QueryClientProvider>,
    );

    // Once the registry loads, "m1" resolves as active — without any click.
    await waitFor(() => expect(screen.getByTestId("active").textContent).toBe("m1"));

    // The local node's scoped data is still cached (node-scoped keys keep each
    // node's entries separate; switching addresses different keys, not eviction).
    expect(qc.getQueryData(["sessions", "local:", "list"])).toEqual(["cached-local-session"]);
  });
});
