import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useNodes } from "./useNodes";

function wrapper(qc: QueryClient) {
  return ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  );
}

beforeEach(() => {
  vi.stubGlobal("fetch", vi.fn(async (input: string) => {
    if (input === "/api/nodes") {
      return new Response(JSON.stringify({ nodes: [
        { id: "local", name: "this", url: "", source: "local", self: true },
        { id: "m1", name: "gpu", url: "http://gpu:80", source: "manual", self: false },
      ] }), { status: 200 });
    }
    if (input === "/api/node/summary") { // local
      return new Response(JSON.stringify({ attention: 2, busy: 1, total: 5 }), { status: 200 });
    }
    if (input === "http://gpu:80/api/node/summary") { // remote down
      return new Response("", { status: 502 });
    }
    return new Response("", { status: 404 });
  }));
});
afterEach(() => vi.unstubAllGlobals());

describe("useNodes", () => {
  it("aggregates summaries and marks unreachable nodes offline", async () => {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const { result } = renderHook(() => useNodes(), { wrapper: wrapper(qc) });

    await waitFor(() => expect(result.current.nodes).toHaveLength(2));
    const local = result.current.nodes.find((n) => n.id === "local")!;
    const gpu = result.current.nodes.find((n) => n.id === "m1")!;

    await waitFor(() => expect(local.summary?.attention).toBe(2));
    expect(local.online).toBe(true);
    await waitFor(() => expect(gpu.online).toBe(false));
    expect(gpu.summary).toBeNull();
  });
});
