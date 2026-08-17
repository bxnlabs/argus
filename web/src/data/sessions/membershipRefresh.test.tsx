import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { cleanup, renderHook, waitFor, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { StubNodeProvider } from "@/test/node-context";
import { sessionKeys, statusKeys } from "./keys";
import { useCreateSession, useCloneSession, useDeleteSession } from "./queries";

const listKey = sessionKeys.list("local:");
const statusKey = statusKeys.all("local:");

beforeEach(() => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => new Response(JSON.stringify({ success: true }), { status: 200 })),
  );
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function wrapper(qc: QueryClient) {
  return ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={qc}>
      <StubNodeProvider>{children}</StubNodeProvider>
    </QueryClientProvider>
  );
}

// Seeds both caches as already-fetched, so "was it invalidated" is observable as
// the entry going stale rather than as a fetch count.
function seed() {
  const qc = new QueryClient();
  qc.setQueryData(listKey, { home_dir: "/home/u", sessions: [{ id: "a" }] });
  qc.setQueryData(statusKey, { statuses: { a: { status: "idle" } } });
  return qc;
}

describe("session membership changes", () => {
  // A membership change that refreshed only the list would leave a session on
  // screen that no status covers — the row falls back to an unlabeled muted dot
  // until the next status tick.
  it.each([
    ["create", () => useCreateSession(), () => ({ provider_type: "claude", auto_approve: false })],
    ["clone", () => useCloneSession(), () => ({ sessionId: "a" })],
    ["delete", () => useDeleteSession(), () => ({ sessionId: "a" })],
  ])("refreshes the statuses too on %s", async (_name, useHook, vars) => {
    const qc = seed();
    expect(qc.getQueryState(statusKey)?.isInvalidated).toBe(false);

    const { result } = renderHook(() => useHook(), { wrapper: wrapper(qc) });
    act(() => {
      (result.current.mutate as (v: unknown) => void)(vars());
    });

    await waitFor(() => {
      expect(qc.getQueryState(statusKey)?.isInvalidated).toBe(true);
      expect(qc.getQueryState(listKey)?.isInvalidated).toBe(true);
    });
  });
});
