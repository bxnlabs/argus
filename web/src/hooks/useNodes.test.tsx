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

    await waitFor(() =>
      expect(result.current.nodes.find((n) => n.id === "local")!.summary?.attention).toBe(2),
    );
    // Wait for the *settled* error, not just online===false — online is also
    // false while the first poll is still pending, so gating on it alone would
    // let the pending assertion below race the 502.
    await waitFor(() => {
      const gpu = result.current.nodes.find((n) => n.id === "m1")!;
      expect(gpu.online).toBe(false);
      expect(gpu.pending).toBe(false);
    });

    // Re-read a fresh snapshot after both polls have settled.
    const local = result.current.nodes.find((n) => n.id === "local")!;
    const gpu = result.current.nodes.find((n) => n.id === "m1")!;

    expect(local.online).toBe(true);
    expect(gpu.summary).toBeNull();
    // A settled (errored) poll is Offline, not Connecting.
    expect(local.pending).toBe(false);
    expect(gpu.pending).toBe(false);
  });

  it("keeps a node offline while its poll is in flight (no online flash)", async () => {
    // Reproduces the rail flicker: a blackholed node's summary fetch hangs until
    // it times out, and React Query reports the in-flight retry with isError
    // false. Gating on !isError flashed the node online for those seconds; gating
    // on isSuccess keeps it offline until a poll actually returns.
    vi.stubGlobal("fetch", vi.fn(async (input: string) => {
      if (input === "/api/nodes") {
        return new Response(JSON.stringify({ nodes: [
          { id: "local", name: "this", url: "", source: "local", self: true },
          { id: "m1", name: "gpu", url: "http://gpu:80", source: "manual", self: false },
        ] }), { status: 200 });
      }
      if (input === "/api/node/summary") {
        return new Response(JSON.stringify({ attention: 0, busy: 0, total: 1 }), { status: 200 });
      }
      if (input === "http://gpu:80/api/node/summary") {
        return new Promise<Response>(() => {}); // hangs, like a blackholed node
      }
      return new Response("", { status: 404 });
    }));

    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const { result } = renderHook(() => useNodes(), { wrapper: wrapper(qc) });

    await waitFor(() => expect(result.current.nodes).toHaveLength(2));
    // Local resolves and reads online — the control.
    await waitFor(() =>
      expect(result.current.nodes.find((n) => n.id === "local")!.online).toBe(true),
    );
    // The hung remote must never report online while its fetch is pending.
    const gpu = result.current.nodes.find((n) => n.id === "m1")!;
    expect(gpu.online).toBe(false);
    expect(gpu.summary).toBeNull();
    // Its first poll has never settled, so it reads as Connecting, not Offline.
    expect(gpu.pending).toBe(true);
  });
});
