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
afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

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

  it("formats a discovered node's tailnet hostname like a manual name", async () => {
    vi.stubGlobal("fetch", vi.fn(async (input: string) => {
      if (input === "/api/nodes") {
        return new Response(JSON.stringify({ nodes: [
          { id: "local", name: "this", url: "", source: "local", self: true },
          { id: "d1", name: "argus-bumblebee.tail06de7.ts.net", url: "http://argus-bumblebee.tail06de7.ts.net", source: "discovered", self: false },
        ] }), { status: 200 });
      }
      return new Response(JSON.stringify({ attention: 0, busy: 0, total: 0 }), { status: 200 });
    }));

    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const { result } = renderHook(() => useNodes(), { wrapper: wrapper(qc) });

    await waitFor(() => expect(result.current.nodes).toHaveLength(2));
    // Discovered hostname collapses to the same friendly name a manual node gets.
    expect(result.current.nodes.find((n) => n.id === "d1")!.name).toBe("bumblebee");
    // Non-discovered names are left exactly as-is.
    expect(result.current.nodes.find((n) => n.id === "local")!.name).toBe("this");
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

  it("flips a node offline when a previously-successful poll later 502s", async () => {
    // Locks the success→fail transition: a node that polled OK and then goes
    // down must read offline with its stale summary cleared. With retry:false a
    // settled failed *background* poll sets status:'error' (isSuccess false) even
    // though the query keeps its last data, so `online = isSuccess` handles this;
    // this guards against a regression to e.g. !isError or "has data ⇒ online".
    //
    // Must use fake timers to drive the real 5s refetchInterval — the production
    // path. A manual refetch()/refetchQueries() races the observer re-render and
    // ABORTS the in-flight poll's signal, so the poll is cancelled (not errored),
    // isSuccess wrongly stays true, and the test would assert the wrong thing.
    vi.useFakeTimers();
    let remoteShouldFail = false;
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
        return remoteShouldFail
          ? new Response("", { status: 502 })
          : new Response(JSON.stringify({ attention: 1, busy: 0, total: 3 }), { status: 200 });
      }
      return new Response("", { status: 404 });
    }));

    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const { result } = renderHook(() => useNodes(), { wrapper: wrapper(qc) });

    // First poll succeeds → online with a summary.
    await vi.advanceTimersByTimeAsync(50);
    const first = result.current.nodes.find((n) => n.id === "m1")!;
    expect(first.online).toBe(true);
    expect(first.summary?.attention).toBe(1);

    // Backend goes down; the next 5s background poll 502s.
    remoteShouldFail = true;
    await vi.advanceTimersByTimeAsync(5000);

    const gpu = result.current.nodes.find((n) => n.id === "m1")!;
    expect(gpu.online).toBe(false);
    // Stale summary is cleared along with the offline flip.
    expect(gpu.summary).toBeNull();
  });

  it("falls back to a local node when the registry request fails", async () => {
    vi.stubGlobal("fetch", vi.fn(async (input: string) => {
      if (input === "/api/nodes") return new Response("", { status: 500 });
      // Same-origin local summary still answers (registry read failed, node up).
      if (input === "/api/node/summary") {
        return new Response(JSON.stringify({ attention: 0, busy: 0, total: 2 }), { status: 200 });
      }
      return new Response("", { status: 404 });
    }));

    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const { result } = renderHook(() => useNodes(), { wrapper: wrapper(qc) });

    // The hook settles (isLoaded) and surfaces the synthetic local node rather
    // than an empty list, so the app stays usable against this machine.
    await waitFor(() => expect(result.current.isLoaded).toBe(true));
    await waitFor(() => expect(result.current.nodes).toHaveLength(1));
    const local = result.current.nodes[0];
    expect(local.id).toBe("local");
    expect(local.self).toBe(true);
    expect(local.url).toBe("");
  });
});
