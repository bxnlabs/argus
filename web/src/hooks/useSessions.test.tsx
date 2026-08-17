import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { StubNodeProvider } from "@/test/node-context";
import { sessionKeys } from "@/data/sessions/keys";
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
let respond: () => Response | Promise<Response>;

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
// A failure that settles on a later macrotask, the way a real one does. With no
// cached data the query's status cycles error → pending → error across a retry,
// and an immediately-resolved failure lets React batch that pending render away
// — so only a deferred failure exercises the window where the re-arm guard can
// wrongly clear itself.
const slowDown = () =>
  new Promise<Response>((r) => setTimeout(() => r(down()), 20));

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
    //
    // The failures are deferred on purpose: a node that has never answered holds
    // no cached data, so each retry drops the status back to "pending" before it
    // errors again. Re-arming on `!isError` would clear the guard in that window
    // and re-announce every poll. Immediately-resolved failures hide it.
    const qc = client();
    respond = slowDown;

    renderHook(() => useSessions(), { wrapper: wrapper(qc) });
    await waitFor(() => expect(toastError).toHaveBeenCalledTimes(1));

    // Settle between polls so React commits the intermediate render rather than
    // batching it away — the poll interval is 10s in production, so the pending
    // state is plainly on screen there.
    for (let i = 0; i < 2; i++) {
      await qc.refetchQueries({ queryKey: ["sessions"] });
      await new Promise((r) => setTimeout(r, 30));
    }

    expect(toastError).toHaveBeenCalledTimes(1);
  });

  // Seeds the cache the way a node switch leaves it: real data from an earlier
  // visit, with no fetch yet made by the mount that's about to read it. Dated
  // back past staleTime so the mount refetches, which is what a return visit
  // after any real interval does.
  const seedRoster = (qc: QueryClient, sessions: unknown[]) =>
    qc.setQueryData(
      sessionKeys.list("local:"),
      { sessions, home_dir: "/home/u" },
      { updatedAt: Date.now() - 60_000 },
    );

  it("draws a warm cache without treating it as a roster answer", async () => {
    // The node-switch shape. isLoaded and isRosterAuthoritative deliberately
    // disagree: the cached list is worth drawing immediately, but nothing has
    // confirmed it still describes the server, so it can't be used to conclude a
    // session is gone. App's stale-tab cleanup detaches on absence, and this
    // list may predate a session created just before the node went quiet.
    const qc = client();
    seedRoster(qc, [{ id: "a", name: "one" }]);
    respond = () => ok([{ id: "a", name: "one" }]);

    const { result } = renderHook(() => useSessions(), { wrapper: wrapper(qc) });

    // Asserted synchronously, before the mount's refetch can settle. This
    // instant is the whole point: it's when App's stale-tab cleanup effect runs,
    // and it's *before* TabProvider has restored tabs from localStorage, since
    // child effects commit before parent ones. Granting authority here consumes
    // the one-shot guard against a tab list that hasn't loaded yet.
    expect(result.current.isLoaded).toBe(true);
    expect(result.current.isRosterAuthoritative).toBe(false);

    // ...and it still arrives once the fetch answers.
    await waitFor(() => expect(result.current.isRosterAuthoritative).toBe(true));
  });

  it("doesn't announce a stale error for a node that has since recovered", async () => {
    // Node goes down, gets announced, and you switch away. The query keeps that
    // error in cache; coming back remounts the hook on top of it with a refetch
    // already in flight. Announcing off the cached error would report a node as
    // down at the moment it is answering.
    // Seeded, because the error has to survive the remount to be stale at all:
    // with data cached the status sits at "error" while the next fetch runs,
    // whereas an empty cache drops to "pending" and hides the window entirely.
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
    // Rename and pin both setQueryData on this key in onMutate, and TanStack
    // scores a manual write as `status: "success"` — so reading `isSuccess`
    // would let renaming one session promote a stale roster to authoritative
    // with no request behind it, and the cleanup would detach a tab holding a
    // session that list predates.
    const qc = client();
    seedRoster(qc, [{ id: "a", name: "one" }]);
    respond = down;

    const { result, rerender } = renderHook(() => useSessions(), { wrapper: wrapper(qc) });
    await waitFor(() => expect(toastError).toHaveBeenCalled());
    expect(result.current.isRosterAuthoritative).toBe(false);

    seedRoster(qc, [{ id: "a", name: "renamed" }]);
    rerender();

    expect(result.current.isRosterAuthoritative).toBe(false);
  });

  it("doesn't let renaming a row during an outage count as recovery", async () => {
    // Re-arming on `isSuccess` would treat an optimistic write as the node
    // coming back, so editing a cached row mid-outage would license a second
    // announcement for the same uninterrupted outage.
    const qc = client();
    seedRoster(qc, [{ id: "a", name: "one" }]);
    respond = down;

    renderHook(() => useSessions(), { wrapper: wrapper(qc) });
    await waitFor(() => expect(toastError).toHaveBeenCalledTimes(1));

    // Settled before the refetch, so React actually commits the render where the
    // optimistic write has the query reading as a success. Without that gap the
    // batching hides the very state the guard has to resist, and this test would
    // pass against the bug.
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
    // The sequence rename and pin actually perform in onMutate: cancel the
    // in-flight roster fetch, then write the cache. Cancelling stops the fetch
    // and the write lands as success, so at the render layer this is
    // indistinguishable from a fetch that answered — the observer goes
    // fetching → idle+success and React batches the cancelled state in between
    // out of existence. Anything reading rendered status grants authority here
    // and detaches a tab for a session the stale roster predates.
    const qc = client();
    seedRoster(qc, [{ id: "a", name: "one" }]);
    respond = slowDown;

    const { result, rerender } = renderHook(() => useSessions(), { wrapper: wrapper(qc) });
    expect(result.current.isRosterAuthoritative).toBe(false);

    await act(async () => {
      await qc.cancelQueries({ queryKey: sessionKeys.list("local:") });
      qc.setQueryData(sessionKeys.list("local:"), {
        sessions: [{ id: "a", name: "renamed" }],
        home_dir: "/home/u",
      });
    });
    rerender();

    expect(result.current.isRosterAuthoritative).toBe(false);
    expect(result.current.isRosterSettled).toBe(false);
  });

  it("won't call a session absent while the roster is unsettled", async () => {
    // What App's dialog-closing effect rides on. A session created seconds ago
    // is missing from the cached list until its invalidating refetch lands, and
    // if that refetch is slow or failing the list still reads as loaded — so
    // "not in the list" has to stay distinguishable from "gone".
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
