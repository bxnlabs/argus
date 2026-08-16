import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { cleanup, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { StubNodeProvider } from "@/test/node-context";
import { useSessions } from "./useSessions";

const toastError = vi.fn();
vi.mock("sonner", () => ({ toast: { error: (...a: unknown[]) => toastError(...a) } }));

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
  toastError.mockClear();
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
const down = () => new Response("", { status: 502 });

const client = () => new QueryClient({ defaultOptions: { queries: { retry: false } } });

describe("useSessions", () => {
  it("keeps the cached list through a failed background refetch", async () => {
    // A failed poll leaves TanStack's status at "error" while the last good data
    // stays cached. Dropping isLoaded here would swap the populated sidebar back
    // to placeholder rows every time a remote node blinked.
    const qc = client();
    respond = () => ok([{ id: "a", name: "one" }]);

    const { result, rerender } = renderHook(() => useSessions(), { wrapper: wrapper(qc) });
    await waitFor(() => expect(result.current.sessions).toHaveLength(1));

    respond = down;
    await qc.refetchQueries({ queryKey: ["sessions"] });
    rerender();

    expect(result.current.sessions).toHaveLength(1);
    expect(result.current.isLoaded).toBe(true);
  });

  it("reports not-loaded when the first fetch fails, so the list shows placeholders", async () => {
    const qc = client();
    respond = down;

    const { result } = renderHook(() => useSessions(), { wrapper: wrapper(qc) });

    await waitFor(() => expect(toastError).toHaveBeenCalled());
    expect(result.current.isLoaded).toBe(false);
    expect(result.current.sessions).toHaveLength(0);
  });

  it("announces a failure once rather than on every failed poll", async () => {
    // An unreachable node keeps polling by design. Toasting per attempt would
    // emit one every 10s for as long as it stayed down.
    const qc = client();
    respond = down;

    renderHook(() => useSessions(), { wrapper: wrapper(qc) });
    await waitFor(() => expect(toastError).toHaveBeenCalledTimes(1));

    await qc.refetchQueries({ queryKey: ["sessions"] });
    await qc.refetchQueries({ queryKey: ["sessions"] });

    expect(toastError).toHaveBeenCalledTimes(1);
  });

  it("announces again after recovering, so a second outage isn't silent", async () => {
    const qc = client();
    respond = down;

    const { rerender } = renderHook(() => useSessions(), { wrapper: wrapper(qc) });
    await waitFor(() => expect(toastError).toHaveBeenCalledTimes(1));

    respond = () => ok([{ id: "a" }]);
    await qc.refetchQueries({ queryKey: ["sessions"] });
    rerender();

    respond = down;
    await qc.refetchQueries({ queryKey: ["sessions"] });
    rerender();

    await waitFor(() => expect(toastError).toHaveBeenCalledTimes(2));
  });
});
