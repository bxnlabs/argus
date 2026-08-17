import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { cleanup, renderHook, waitFor, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { StubNodeProvider } from "@/test/node-context";
import { sessionKeys } from "./keys";
import { useRosterFetchState, useSessionsQuery } from "./queries";

const listKey = sessionKeys.list("local:");

function wrapper(qc: QueryClient) {
  return ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={qc}>
      <StubNodeProvider>{children}</StubNodeProvider>
    </QueryClientProvider>
  );
}

const ok = (sessions: unknown[]) =>
  new Response(JSON.stringify({ sessions, home_dir: "/home/u" }), { status: 200 });

beforeEach(() => {
  vi.stubGlobal("fetch", vi.fn(async () => ok([{ id: "a", name: "one" }])));
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const client = () => new QueryClient({ defaultOptions: { queries: { retry: false } } });

describe("useRosterFetchState", () => {
  it("separates 'an answer arrived' from 'the roster is that answer'", async () => {
    // The invariant the stale-tab cleanup's gate rests on, and the reason it
    // asks `settled` rather than anything latched.
    //
    // A failed rename or pin restores the roster it snapshotted in onMutate,
    // which is older than any answer that landed while the mutation was in
    // flight. After that write these two facts genuinely disagree: a real
    // answer *did* arrive (the revision counter still records it, which is what
    // lets the outage toast re-arm on the event), and the roster on hand is no
    // longer that answer.
    //
    // Anything that deletes on absence has to read the second one. A latched
    // "a fetch has answered" flag stays true straight through the rollback, so
    // a one-shot consumer can spend its only pass judging "this session is
    // gone" against a list the cache has already reverted.
    // The server has since learned about a second session; the rollback below
    // puts back a snapshot taken before it existed, which is what makes the
    // reverted roster genuinely older rather than merely rewritten.
    const qc = client();
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => ok([{ id: "a", name: "one" }, { id: "b", name: "two" }])),
    );
    const { result, rerender } = renderHook(
      () => {
        useSessionsQuery();
        return useRosterFetchState();
      },
      { wrapper: wrapper(qc) },
    );

    await waitFor(() => expect(result.current.settled).toBe(true));
    const answered = result.current.fetchedRevision;
    expect(answered).toBeGreaterThan(0);

    // onError's rollback: the pre-mutation roster written back over a newer one.
    // Session "b" vanishes from the cache despite the server having reported it.
    act(() => {
      qc.setQueryData(listKey, {
        sessions: [{ id: "a", name: "one" }],
        home_dir: "/home/u",
      });
    });
    rerender();

    // The divergence. Read the wrong one and you delete against stale data.
    expect(result.current.fetchedRevision).toBe(answered);
    expect(result.current.settled).toBe(false);
  });

  it("comes back once a real answer lands on top of the local write", async () => {
    // The rollback must delay the absence check, not disable it — otherwise a
    // stale tab from a previous run never gets cleaned up at all.
    const qc = client();
    const { result, rerender } = renderHook(
      () => {
        useSessionsQuery();
        return useRosterFetchState();
      },
      { wrapper: wrapper(qc) },
    );

    await waitFor(() => expect(result.current.settled).toBe(true));
    act(() => {
      qc.setQueryData(listKey, { sessions: [], home_dir: "/home/u" });
    });
    rerender();
    expect(result.current.settled).toBe(false);

    await act(async () => {
      await qc.refetchQueries({ queryKey: listKey });
    });
    rerender();

    expect(result.current.settled).toBe(true);
  });
});
