import { describe, it, expect, vi, afterEach } from "vitest";
import {
  apiFetch,
  apiTextFetch,
  nodeWsUrl,
  ApiError,
  TimeoutError,
  POLL_TIMEOUT_MS,
  REQUEST_TIMEOUT_MS,
} from "./client";

afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

describe("nodeWsUrl", () => {
  it("targets a remote node's origin with a ws scheme", () => {
    expect(nodeWsUrl("http://gpu-box:80", "s1")).toBe(
      "ws://gpu-box:80/api/node/ws/sessions/s1",
    );
  });

  it("falls back to same-origin when baseUrl is empty", () => {
    // jsdom serves the test from http://localhost (see vite.config test env).
    expect(nodeWsUrl("", "s1")).toBe("ws://localhost/api/node/ws/sessions/s1");
    expect(nodeWsUrl("")).toBe("ws://localhost/api/node/ws/terminal");
  });
});

// A fetch that honours its abort signal the way the real one does, and
// otherwise never settles — a blackholed connection, which is the case a
// deadline exists for. A request that merely fails is already handled.
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

describe("apiFetch deadline", () => {
  it("gives up on a request the node never answers", async () => {
    vi.useFakeTimers();
    vi.stubGlobal("fetch", blackholeFetch());

    const pending = apiFetch("", "/api/node/sessions", { timeoutMs: 500 });
    const assertion = expect(pending).rejects.toBeInstanceOf(TimeoutError);
    await vi.advanceTimersByTimeAsync(600);
    await assertion;
  });

  it("reports the deadline as a timeout, not an anonymous abort", async () => {
    vi.useFakeTimers();
    vi.stubGlobal("fetch", blackholeFetch());

    const pending = apiFetch("", "/api/node/sessions", { timeoutMs: 500 }).catch(
      (e) => e,
    );
    await vi.advanceTimersByTimeAsync(600);
    const err = await pending;

    // The DOMException an abort raises can't say which abort it was, which is
    // what makes "the node never answered" indistinguishable from "the caller
    // cancelled" unless the deadline names itself.
    expect(err).toBeInstanceOf(TimeoutError);
    expect((err as TimeoutError).timeoutMs).toBe(500);
  });

  it("leaves a request that answers in time alone", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response(JSON.stringify({ ok: true }), { status: 200 })),
    );

    await expect(apiFetch("", "/api/node/sessions")).resolves.toEqual({ ok: true });
  });

  it("still surfaces a real HTTP failure as ApiError", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response("", { status: 502 })),
    );

    await expect(apiFetch("", "/api/node/sessions")).rejects.toBeInstanceOf(ApiError);
  });

  it("passes the caller's own abort through rather than swallowing it", async () => {
    vi.stubGlobal("fetch", blackholeFetch());

    const caller = new AbortController();
    const pending = apiFetch("", "/api/node/sessions", {
      signal: caller.signal,
    }).catch((e) => e);
    caller.abort();
    const err = await pending;

    // Aborting for the caller's own reasons must not be reported as the node
    // failing to answer — the sidebar announces one of those and not the other.
    expect(err).not.toBeInstanceOf(TimeoutError);
  });

  it("keeps the deadline when the caller also brings a signal", async () => {
    // With `AbortSignal.any` removed, which is the case the old composition
    // fell back for: it picked the caller's signal alone, so supplying one
    // silently deleted the deadline. Both aborts now run through a single
    // controller, so there is nothing to fall back to and nothing to drop.
    //
    // jsdom does provide `AbortSignal.any`, so testing this without removing it
    // would pass against the very code it is meant to reject.
    vi.useFakeTimers();
    vi.stubGlobal("fetch", blackholeFetch());
    const realAny = AbortSignal.any;
    // @ts-expect-error — deliberately simulating a runtime without it.
    delete AbortSignal.any;

    try {
      const caller = new AbortController();
      const pending = apiFetch("", "/api/node/sessions", {
        signal: caller.signal,
        timeoutMs: 500,
      }).catch((e) => e);
      await vi.advanceTimersByTimeAsync(600);

      expect(await pending).toBeInstanceOf(TimeoutError);
    } finally {
      AbortSignal.any = realAny;
    }
  });

  it("covers a response that arrives and then stalls mid-body", async () => {
    // `fetch` resolves on headers, so a deadline that ends there guards nothing
    // that matters: a node which sends 200 and then stops writing hangs exactly
    // as long as one that never answered. The stream below never closes.
    vi.useFakeTimers();
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async (_url: string, init?: RequestInit) =>
          new Response(
            new ReadableStream({
              start(controller) {
                init?.signal?.addEventListener("abort", () =>
                  controller.error(new DOMException("aborted", "AbortError")),
                );
              },
            }),
            { status: 200, headers: { "Content-Type": "application/json" } },
          ),
      ),
    );

    const pending = apiFetch("", "/api/node/sessions", { timeoutMs: 500 }).catch(
      (e) => e,
    );
    await vi.advanceTimersByTimeAsync(600);

    expect(await pending).toBeInstanceOf(TimeoutError);
  });

  it("covers a text response that arrives and then stalls mid-body", async () => {
    // The same hole as the JSON case above, and the one that mattered in
    // practice: apiTextFetch used to hand back the Response with its body
    // unread, so the drain happened after the timer was already cleared. A node
    // that sent 200 for a file and then stopped writing left the editor loading
    // forever, with no error and no retry — the loading bookkeeping swallows a
    // second attempt at the same path.
    vi.useFakeTimers();
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async (_url: string, init?: RequestInit) =>
          new Response(
            new ReadableStream({
              start(controller) {
                init?.signal?.addEventListener("abort", () =>
                  controller.error(new DOMException("aborted", "AbortError")),
                );
              },
            }),
            { status: 200 },
          ),
      ),
    );

    const pending = apiTextFetch("", "/api/node/files/content?path=x", {
      timeoutMs: 500,
    }).catch((e) => e);
    await vi.advanceTimersByTimeAsync(600);

    expect(await pending).toBeInstanceOf(TimeoutError);
  });

  it("returns the text body rather than a response still holding it", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response("file contents", { status: 200 })),
    );

    await expect(apiTextFetch("", "/api/node/files/content?path=x")).resolves.toBe(
      "file contents",
    );
  });

  it("doesn't relabel a caller's abort as a timeout when the timer lands after it", async () => {
    // The caller cancels just under the wire and the deadline fires while their
    // rejection is still propagating. Reporting that as TimeoutError tells the
    // app the node failed to answer when the app is the one that hung up —
    // useExpandableDiff records that as a transient anchor failure rather than
    // ignoring it as the cancellation it was.
    // The rejection has to still be in flight when the timer lands, which is
    // the whole race. An immediately-rejecting stub asserts nothing: microtasks
    // drain before the next timer task, so the catch has already run and read
    // the flag by the time the deadline fires.
    //
    // The 5ms delay below makes the rejection a later timer task, which is a
    // stronger delay than native `fetch` is known to take — so read this as an
    // invariant guard on the flag's meaning rather than as proof that the race
    // is reachable through a browser. It does discriminate: without the guard
    // the deadline relabels the caller's abort as a TimeoutError.
    vi.useFakeTimers();
    vi.stubGlobal(
      "fetch",
      vi.fn(
        (_url: string, init?: RequestInit) =>
          new Promise<Response>((_resolve, reject) => {
            init?.signal?.addEventListener("abort", () => {
              setTimeout(() => reject(new DOMException("aborted", "AbortError")), 5);
            });
          }),
      ),
    );

    const caller = new AbortController();
    const pending = apiFetch("", "/api/node/sessions", {
      signal: caller.signal,
      timeoutMs: 500,
    }).catch((e) => e);

    await vi.advanceTimersByTimeAsync(499);
    caller.abort();
    // Past the deadline, so the timer fires while the caller's abort is still
    // making its way out of the fetch.
    await vi.advanceTimersByTimeAsync(10);

    const err = await pending;
    expect(err).not.toBeInstanceOf(TimeoutError);
    expect(err).toBeInstanceOf(DOMException);
  });

  it("runs with no deadline at all when asked for none", async () => {
    // For the handlers nothing bounds: `git clone` runs through a bare
    // exec.Command on the node, so any ceiling here is a guess about someone
    // else's repo, and guessing low reports failure for a create that succeeds.
    vi.useFakeTimers();
    vi.stubGlobal("fetch", blackholeFetch());

    let settled = false;
    const pending = apiFetch("", "/api/node/sessions", { timeoutMs: null })
      .catch(() => {})
      .finally(() => {
        settled = true;
      });

    // Well past the default, which is what would otherwise have fired.
    await vi.advanceTimersByTimeAsync(REQUEST_TIMEOUT_MS * 2);
    expect(settled).toBe(false);

    vi.useRealTimers();
    void pending;
  });

  it("still honours the caller's abort with no deadline set", async () => {
    // Opting out of the deadline must not opt out of cancellation — the caller
    // is the only thing left that can stop the request.
    vi.stubGlobal("fetch", blackholeFetch());

    const caller = new AbortController();
    const pending = apiFetch("", "/api/node/sessions", {
      timeoutMs: null,
      signal: caller.signal,
    }).catch((e) => e);
    caller.abort();

    const err = await pending;
    expect(err).toBeInstanceOf(DOMException);
    expect(err).not.toBeInstanceOf(TimeoutError);
  });

  it("keeps the polled reads on a much shorter leash than the default", () => {
    // The polls are DB reads that repeat on a timer; the default ceiling is
    // sized for the node's slowest git work. Collapsing the two would mean a
    // black hole holds the poll loop open long enough that nothing downstream
    // can report the node stopped answering.
    expect(POLL_TIMEOUT_MS).toBeLessThan(REQUEST_TIMEOUT_MS);
    expect(POLL_TIMEOUT_MS).toBeLessThan(10_000);
  });
});
