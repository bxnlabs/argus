import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { cleanup, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { StubNodeProvider } from "@/test/node-context";
import { useSessions } from "./useSessions";

function wrapper(qc: QueryClient) {
  return ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={qc}>
      <StubNodeProvider>{children}</StubNodeProvider>
    </QueryClientProvider>
  );
}

// Each call decides the next response, so a test can serve a good list first and
// fail everything after it — the shape of a node that goes unreachable mid-poll.
let respond: () => Response;

beforeEach(() => {
  vi.stubGlobal("fetch", vi.fn(async () => respond()));
});
// Explicit unmount: the suite doesn't run with Vitest globals, so Testing
// Library never registers its automatic cleanup. Leaving the hook mounted would
// leave a live query observer polling on a 10s interval into the next test —
// and past the point where fetch is still stubbed.
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const ok = (sessions: unknown[]) =>
  new Response(JSON.stringify({ sessions, home_dir: "/home/u" }), { status: 200 });

describe("useSessions", () => {
  it("keeps the cached list through a failed background refetch", async () => {
    // The list polls every 10s. A failed poll leaves TanStack's status at
    // "error" while the last good data stays in cache, so deriving readiness
    // from isSuccess/isError would blank the sidebar and swap in a retry screen
    // on every transient blip — with usable rows sitting right there.
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    respond = () => ok([{ id: "a", name: "one" }]);

    const { result, rerender } = renderHook(() => useSessions(), { wrapper: wrapper(qc) });
    await waitFor(() => expect(result.current.sessions).toHaveLength(1));

    respond = () => new Response("", { status: 502 });
    await qc.refetchQueries({ queryKey: ["sessions"] });
    rerender();

    expect(result.current.sessions).toHaveLength(1);
    expect(result.current.isLoaded).toBe(true);
    expect(result.current.isError).toBe(false);
    expect(result.current.errorMessage).toBeUndefined();
  });

  it("reports an error only when the first fetch fails with nothing cached", async () => {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    respond = () => new Response("", { status: 502 });

    const { result } = renderHook(() => useSessions(), { wrapper: wrapper(qc) });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.isLoaded).toBe(false);
    expect(result.current.sessions).toHaveLength(0);
  });
});
