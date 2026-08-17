import { describe, it, expect, vi, afterEach } from "vitest";
import {
  apiFetch,
  nodeWsUrl,
  ApiError,
  TimeoutError,
  POLL_TIMEOUT_MS,
  REQUEST_TIMEOUT_MS,
  SESSION_OPERATION_TIMEOUT_MS,
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

  it("gives the docker-capable session operations room the node actually needs", () => {
    // The node allows `docker compose up` 20 minutes (stackOpTimeout), and
    // create/clone/profile-change can all sit on one. A ceiling below that
    // reports failure for work that is still running and will succeed — worse
    // than no ceiling, because the user retries and creates a duplicate.
    expect(SESSION_OPERATION_TIMEOUT_MS).toBeGreaterThan(20 * 60_000);
    expect(REQUEST_TIMEOUT_MS).toBeLessThan(SESSION_OPERATION_TIMEOUT_MS);
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
