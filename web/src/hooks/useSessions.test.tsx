import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { StubNodeProvider } from "@/test/node-context";
import { sessionKeys } from "@/data/sessions/keys";
import { POLL_TIMEOUT_MS } from "@/api/client";
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

// Each call decides the next response, so a test can serve a good list and then
// fail — the shape of a node that goes unreachable mid-poll.
let respond: () => Response | Promise<Response>;

beforeEach(() => {
  toastError.mockClear();
  vi.stubGlobal("fetch", vi.fn(async () => respond()));
});

// Explicit cleanup: without Vitest globals, Testing Library registers none. A
// mounted hook would keep a query observer polling into the next test, past the
// point where fetch is still stubbed.
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const ok = (sessions: unknown[]) =>
  new Response(JSON.stringify({ sessions, home_dir: "/home/u" }), { status: 200 });
const down = () => new Response("", { status: 502 });
// A failure that settles on a later macrotask, the way a real one does. With no
// cached data the status cycles error → pending → error across a retry, and only
// a deferred failure lets React commit that pending render.
const slowDown = () =>
  new Promise<Response>((r) => setTimeout(() => r(down()), 20));

const client = () => new QueryClient({ defaultOptions: { queries: { retry: false } } });

describe("useSessions", () => {
  it("keeps the cached list through a failed background refetch", async () => {
    // A failed poll leaves the status at "error" while the last good data stays
    // cached; the sidebar shouldn't fall back to placeholders when a node blinks.
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
    // An unreachable node keeps polling by design; toasting per attempt would emit
    // one every 10s. Failures are deferred so the "pending" render commits.
    const qc = client();
    respond = slowDown;

    renderHook(() => useSessions(), { wrapper: wrapper(qc) });
    await waitFor(() => expect(toastError).toHaveBeenCalledTimes(1));

    // Settle between polls so React commits the intermediate render; with a 10s
    // interval in production that render is plainly on screen.
    for (let i = 0; i < 2; i++) {
      await qc.refetchQueries({ queryKey: ["sessions"] });
      await new Promise((r) => setTimeout(r, 30));
    }

    expect(toastError).toHaveBeenCalledTimes(1);
  });

  // Seeds the cache the way a node switch leaves it: data from an earlier visit,
  // with no fetch yet by the mount about to read it, dated past staleTime so that
  // mount refetches.
  const seedRoster = (qc: QueryClient, sessions: unknown[]) =>
    qc.setQueryData(
      sessionKeys.list("local:"),
      { sessions, home_dir: "/home/u" },
      { updatedAt: Date.now() - 60_000 },
    );

  it("draws a warm cache without treating it as a roster answer", async () => {
    // The node-switch shape: the cached list is worth drawing, but nothing has
    // confirmed it still describes the server, and App's cleanup detaches on
    // absence.
    const qc = client();
    seedRoster(qc, [{ id: "a", name: "one" }]);
    respond = () => ok([{ id: "a", name: "one" }]);

    const { result } = renderHook(() => useSessions(), { wrapper: wrapper(qc) });

    // Asserted synchronously, before the mount's refetch can settle: that is when
    // App's stale-tab cleanup runs, and before TabProvider has restored tabs from
    // localStorage, since child effects commit before parent ones.
    expect(result.current.isLoaded).toBe(true);
    expect(result.current.isRosterSettled).toBe(false);

    // ...and it still arrives once the fetch answers.
    await waitFor(() => expect(result.current.isRosterSettled).toBe(true));
  });

  it("doesn't announce a stale error for a node that has since recovered", async () => {
    // Node goes down, gets announced, and you switch away; coming back remounts
    // the hook on the cached error with a refetch already in flight. Announcing
    // off that error would report a node as down at the moment it is answering.
    // Seeded so the status sits at "error" while that fetch runs, rather than
    // dropping to "pending" as an empty cache would.
    const qc = client();
    seedRoster(qc, [{ id: "a", name: "one" }]);
    respond = down;

    const first = renderHook(() => useSessions(), { wrapper: wrapper(qc) });
    await waitFor(() => expect(toastError).toHaveBeenCalledTimes(1));
    first.unmount();

    respond = () => ok([{ id: "a", name: "one" }, { id: "b", name: "two" }]);
    const second = renderHook(() => useSessions(), { wrapper: wrapper(qc) });
    await waitFor(() => expect(second.result.current.sessions).toHaveLength(2));

    expect(toastError).toHaveBeenCalledTimes(1);
  });

  it("still announces on remount when the node really is still down", async () => {
    // The other half: holding the toast while a fetch is in flight must delay
    // the announcement, not swallow it.
    const qc = client();
    seedRoster(qc, [{ id: "a", name: "one" }]);
    respond = down;

    const first = renderHook(() => useSessions(), { wrapper: wrapper(qc) });
    await waitFor(() => expect(toastError).toHaveBeenCalledTimes(1));
    first.unmount();

    renderHook(() => useSessions(), { wrapper: wrapper(qc) });

    await waitFor(() => expect(toastError).toHaveBeenCalledTimes(2));
  });

  it("doesn't let an optimistic cache write pass for a server answer", async () => {
    // Rename and pin both setQueryData on this key in onMutate, and a manual write
    // scores as `status: "success"` — but nothing requested it.
    const qc = client();
    seedRoster(qc, [{ id: "a", name: "one" }]);
    respond = down;

    const { result, rerender } = renderHook(() => useSessions(), { wrapper: wrapper(qc) });
    await waitFor(() => expect(toastError).toHaveBeenCalled());
    expect(result.current.isRosterSettled).toBe(false);

    seedRoster(qc, [{ id: "a", name: "renamed" }]);
    rerender();

    expect(result.current.isRosterSettled).toBe(false);
  });

  it("announces a node that accepts the connection and then says nothing", async () => {
    // The failure the deadline exists for: a swallowed request never settles, so
    // without one the query would sit fetching forever — no toast, no more polls.
    vi.useFakeTimers();
    vi.stubGlobal(
      "fetch",
      vi.fn(
        (_url: string, init?: RequestInit) =>
          new Promise<Response>((_resolve, reject) => {
            init?.signal?.addEventListener("abort", () =>
              reject(new DOMException("aborted", "AbortError")),
            );
          }),
      ),
    );

    const qc = client();
    renderHook(() => useSessions(), { wrapper: wrapper(qc) });

    expect(toastError).not.toHaveBeenCalled();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(POLL_TIMEOUT_MS + 500);
    });

    expect(toastError).toHaveBeenCalledTimes(1);
    vi.useRealTimers();
  });

  it("still hears a recovery that a following write renders over", async () => {
    // A poll that succeeds and is immediately followed by a mutation's optimistic
    // write renders once, as not-settled, so reading that state would miss the
    // recovery and leave the next real outage unannounced.
    const qc = client();
    respond = down;

    renderHook(() => useSessions(), { wrapper: wrapper(qc) });
    await waitFor(() => expect(toastError).toHaveBeenCalledTimes(1));

    respond = () => ok([{ id: "a", name: "one" }]);
    await act(async () => {
      await qc.refetchQueries({ queryKey: ["sessions"] });
      qc.setQueryData(sessionKeys.list("local:"), {
        sessions: [{ id: "a", name: "renamed" }],
        home_dir: "/home/u",
      });
      await new Promise((r) => setTimeout(r, 20));
    });

    respond = down;
    await act(async () => {
      await qc.refetchQueries({ queryKey: ["sessions"] });
      await new Promise((r) => setTimeout(r, 30));
    });

    expect(toastError).toHaveBeenCalledTimes(2);
  });

  it("doesn't let renaming a row during an outage count as recovery", async () => {
    // An optimistic write is not the node coming back: editing a cached row
    // mid-outage must not license a second toast for the same outage.
    const qc = client();
    seedRoster(qc, [{ id: "a", name: "one" }]);
    respond = down;

    renderHook(() => useSessions(), { wrapper: wrapper(qc) });
    await waitFor(() => expect(toastError).toHaveBeenCalledTimes(1));

    // Settled before the refetch so React commits the render where the optimistic
    // write has the query reading as a success; batching would hide it.
    await act(async () => {
      qc.setQueryData(sessionKeys.list("local:"), {
        sessions: [{ id: "a", name: "renamed" }],
        home_dir: "/home/u",
      });
      await new Promise((r) => setTimeout(r, 20));
    });

    await qc.refetchQueries({ queryKey: ["sessions"] });
    await new Promise((r) => setTimeout(r, 30));

    expect(toastError).toHaveBeenCalledTimes(1);
  });

  it("doesn't mistake a cancelled fetch plus a local write for an answer", async () => {
    // The sequence rename and pin perform in onMutate: cancel the in-flight roster
    // fetch, then write the cache. At the render layer that is indistinguishable
    // from a fetch that answered — fetching → idle+success — yet none did.
    const qc = client();
    seedRoster(qc, [{ id: "a", name: "one" }]);
    respond = slowDown;

    const { result, rerender } = renderHook(() => useSessions(), { wrapper: wrapper(qc) });
    expect(result.current.isRosterSettled).toBe(false);

    await act(async () => {
      await qc.cancelQueries({ queryKey: sessionKeys.list("local:") });
      qc.setQueryData(sessionKeys.list("local:"), {
        sessions: [{ id: "a", name: "renamed" }],
        home_dir: "/home/u",
      });
    });
    rerender();

    expect(result.current.isRosterSettled).toBe(false);
  });

  it("stops calling the roster trustworthy once a rollback overwrites it", async () => {
    // A failed rename/pin restores the roster it snapshotted in onMutate, older
    // than any answer that landed while the mutation was in flight: session "b" is
    // on the server and gone from the cache after the rollback.
    //
    // That rules out a latched "a fetch answered at some point" flag, which stays
    // true across the rollback: a one-shot consumer like App's stale-tab cleanup
    // could spend its single pass judging absence against a reverted list, and
    // detach a live tab for "b".
    //
    // A guard on a premise, not a reproduction: it pins what App's cleanup rests on.
    const qc = client();
    respond = () => ok([{ id: "a", name: "one" }, { id: "b", name: "two" }]);

    const { result, rerender } = renderHook(() => useSessions(), { wrapper: wrapper(qc) });
    await waitFor(() => expect(result.current.isRosterSettled).toBe(true));
    expect(result.current.sessions).toHaveLength(2);

    // The rollback: onError's setQueryData putting the pre-mutation roster back.
    act(() => {
      qc.setQueryData(sessionKeys.list("local:"), {
        sessions: [{ id: "a", name: "one" }],
        home_dir: "/home/u",
      });
    });
    rerender();

    expect(result.current.isRosterSettled).toBe(false);
  });

  it("won't call a session absent while the roster is unsettled", async () => {
    // What App's dialog-closing effect rides on: a session created seconds ago is
    // missing from the cached list until its refetch lands, and a slow or failing
    // refetch still reads as loaded, so "not in the list" isn't "gone".
    const qc = client();
    seedRoster(qc, [{ id: "a", name: "one" }]);
    respond = slowDown;

    const { result, rerender } = renderHook(() => useSessions(), { wrapper: wrapper(qc) });

    // In flight: the list is drawable but can't answer the question yet.
    expect(result.current.isLoaded).toBe(true);
    expect(result.current.isRosterSettled).toBe(false);

    // Landed on a failure: still can't.
    await waitFor(() => expect(toastError).toHaveBeenCalled());
    rerender();
    expect(result.current.isRosterSettled).toBe(false);

    // Landed on an answer: now it can.
    respond = () => ok([{ id: "a", name: "one" }]);
    await qc.refetchQueries({ queryKey: ["sessions"] });
    rerender();
    expect(result.current.isRosterSettled).toBe(true);
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
