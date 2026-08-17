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

// Which deadline a mutation actually runs under, observed the only way that
// can't drift from the truth: by letting the clock pass the boundary and seeing
// whether the request is still alive. Asserting on the exported constants would
// pass just as happily with the call site wired to the wrong one — which is the
// mistake these guard, since every one of these numbers was moved by hand.
function wrapper(qc: QueryClient) {
  return ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={qc}>
      <StubNodeProvider>{children}</StubNodeProvider>
    </QueryClientProvider>
  );
}

// Honours abort, never answers otherwise: the blackholed node a deadline exists
// for, and the only case where the deadline is observable at all.
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
  // Every *long-running lifecycle* mutation. Rename, pin and the read-marks are
  // mutations too and deliberately keep the default deadline — they are single
  // DB writes with nothing in them that can outlive a git fetch.
  //
  // The rule for the ones below is that a ceiling under the real work tells the
  // user a mutation failed while the node commits it, and aborting the fetch
  // stops none of it. Each reaches something nothing on the node bounds: a
  // remote source's `git clone` for create, git probes for clone, compose calls
  // bounded only one at a time for a dockerized profile change, worktree
  // removal and branch deletion for delete, and session or profile lock waits
  // for all of them.
  //
  // Listed exhaustively within that group rather than sampled, because the
  // failure this guards against is one call site keeping a ceiling the others
  // dropped, which is invisible in review and only shows up on the slow repo
  // nobody tests with.
  it.each([
    ["create", () => useCreateSession(), { provider_type: "claude", auto_approve: false }],
    ["clone", () => useCloneSession(), { sessionId: "a" }],
    ["profile change", () => useChangeSessionProfile(), { sessionId: "a", profile: "work" }],
    ["profile clear", () => useChangeSessionProfile(), { sessionId: "a", profile: null }],
    ["delete", () => useDeleteSession(), { sessionId: "a" }],
    ["delete with branch", () => useDeleteSession(), { sessionId: "a", deleteBranch: true }],
  ])("leaves %s running with no deadline at all", async (_name, useHook, vars) => {
    // Advanced well past every ceiling these call sites have ever carried — the
    // 75s default, the 120s lifecycle cutoff, the 25-minute session-operation
    // one — so a call site left on any of them fails here rather than sitting
    // inside it and passing either way.
    const state = launch(useHook as never, vars);

    await vi.advanceTimersByTimeAsync(30 * 60_000);
    expect(state.settled).toBe(false);

    // Outliving 30 minutes is not the same fact as carrying no deadline — a
    // freshly-introduced 31-minute ceiling would satisfy the above. This is the
    // exact one: `withDeadline` only builds a controller when it has a timer to
    // hang off it, so with no caller signal either, a request that opted out
    // reaches `fetch` with no signal at all. Any numeric deadline produces one.
    const calls = (globalThis.fetch as unknown as { mock: { calls: [string, RequestInit][] } })
      .mock.calls;
    expect(calls.length).toBeGreaterThan(0);
    expect(calls[calls.length - 1][1]?.signal).toBeUndefined();
  });

  it("still cuts the roster poll, which the rule above does not reach", async () => {
    // The boundary. Dropping deadlines is right for mutations because the node
    // keeps working after the abort; a read is the opposite — the request *is*
    // the work, nothing survives the caller giving up, and a poll left hanging
    // takes the failure toast and the presence dot down with it. Guards against
    // the rule being applied by search-and-replace.
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const { result } = renderHook(() => useSessionsQuery(), { wrapper: wrapper(qc) });

    // Straddled rather than overshot, so this pins POLL_TIMEOUT_MS itself and
    // not merely "something under nine seconds" — the poll being quietly moved
    // onto the 75s default would still fail here.
    await vi.advanceTimersByTimeAsync(POLL_TIMEOUT_MS - 100);
    expect(result.current.error).toBeNull();

    await vi.advanceTimersByTimeAsync(200);
    expect(result.current.error).toBeInstanceOf(TimeoutError);
  });
});
