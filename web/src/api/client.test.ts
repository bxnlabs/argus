import { describe, it, expect, vi, afterEach } from "vitest";
import {
  apiFetch,
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

  it("keeps the polled reads on a much shorter leash than the default", () => {
    // The polls are DB reads that repeat on a timer; the default ceiling is
    // sized for the node's slowest git work. Collapsing the two would mean a
    // black hole holds the poll loop open long enough that nothing downstream
    // can report the node stopped answering.
    expect(POLL_TIMEOUT_MS).toBeLessThan(REQUEST_TIMEOUT_MS);
    expect(POLL_TIMEOUT_MS).toBeLessThan(10_000);
  });
});
