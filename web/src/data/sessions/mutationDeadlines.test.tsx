import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { cleanup, renderHook } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { StubNodeProvider } from "@/test/node-context";
import { TimeoutError, POLL_TIMEOUT_MS } from "@/api/client";
import {
  useCreateSession,
  useCloneSession,
  useDeleteSession,
  useChangeSessionProfile,
  useSessionsQuery,
} from "./queries";

// Which deadline a mutation runs under, observed by letting the clock pass the
// boundary rather than by asserting on the exported constants.
function wrapper(qc: QueryClient) {
  return ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={qc}>
      <StubNodeProvider>{children}</StubNodeProvider>
    </QueryClientProvider>
  );
}

// Honours abort, never answers otherwise: the hung node a deadline exists for.
function blackholeFetch() {
  return vi.fn(
    (_url: string, init?: RequestInit) =>
      new Promise<Response>((_resolve, reject) => {
        init?.signal?.addEventListener("abort", () =>
          reject(new DOMException("aborted", "AbortError")),
        );
      }),
  );
}

beforeEach(() => {
  vi.useFakeTimers();
  vi.stubGlobal("fetch", blackholeFetch());
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

const client = () => new QueryClient();

/** Fires the mutation and reports how it settles, without an unhandled rejection. */
function launch(useHook: () => { mutateAsync: (v: never) => Promise<unknown> }, vars: unknown) {
  const qc = client();
  const { result } = renderHook(() => useHook(), { wrapper: wrapper(qc) });
  const state: { settled: boolean; error?: unknown } = { settled: false };
  void (result.current.mutateAsync as (v: unknown) => Promise<unknown>)(vars).then(
    () => {
      state.settled = true;
    },
    (e) => {
      state.settled = true;
      state.error = e;
    },
  );
  return state;
}

describe("session mutation deadlines", () => {
  // Every long-running lifecycle mutation, listed exhaustively so one call site
  // keeping a ceiling the others dropped fails here. A ceiling under the real
  // work reports a failure while the node commits anyway, and each of these
  // reaches something nothing on the node bounds: a remote source's `git clone`
  // for create, git probes for clone, compose calls bounded only one at a time
  // for a dockerized profile change, worktree removal and branch deletion for
  // delete, and lock waits for all of them. Rename and pin keep the default.
  it.each([
    ["create", () => useCreateSession(), { provider_type: "claude", auto_approve: false }],
    ["clone", () => useCloneSession(), { sessionId: "a" }],
    ["profile change", () => useChangeSessionProfile(), { sessionId: "a", profile: "work" }],
    ["profile clear", () => useChangeSessionProfile(), { sessionId: "a", profile: null }],
    ["delete", () => useDeleteSession(), { sessionId: "a" }],
    ["delete with branch", () => useDeleteSession(), { sessionId: "a", deleteBranch: true }],
  ])("leaves %s running with no deadline at all", async (_name, useHook, vars) => {
    // Far past any deadline a call site could plausibly carry.
    const state = launch(useHook as never, vars);

    await vi.advanceTimersByTimeAsync(30 * 60_000);
    expect(state.settled).toBe(false);

    // Outliving 30 minutes is not the same fact as carrying no deadline. Any
    // numeric deadline builds an abort controller, so only a request that opted
    // out reaches `fetch` with no signal at all.
    const calls = (globalThis.fetch as unknown as { mock: { calls: [string, RequestInit][] } })
      .mock.calls;
    expect(calls.length).toBeGreaterThan(0);
    expect(calls[calls.length - 1][1]?.signal).toBeUndefined();
  });

  it("still cuts the roster poll, which the rule above does not reach", async () => {
    // The boundary: a mutation's work outlives the abort, but for a poll the
    // request is the work, and one left hanging stops anything downstream
    // noticing the node went quiet.
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const { result } = renderHook(() => useSessionsQuery(), { wrapper: wrapper(qc) });

    // Straddled rather than overshot, so this pins POLL_TIMEOUT_MS itself.
    await vi.advanceTimersByTimeAsync(POLL_TIMEOUT_MS - 100);
    expect(result.current.error).toBeNull();

    await vi.advanceTimersByTimeAsync(200);
    expect(result.current.error).toBeInstanceOf(TimeoutError);
  });
});
