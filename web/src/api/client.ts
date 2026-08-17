export class ApiError extends Error {
  constructor(
    message: string,
    public status: number,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

/** A request that passed its deadline without the server answering. */
export class TimeoutError extends Error {
  constructor(public timeoutMs: number) {
    super(`Request timed out after ${timeoutMs}ms`);
    this.name = "TimeoutError";
  }
}

/**
 * Default request deadline, for the ordinary read.
 *
 * It clears the slowest bound that work carries: the git layer caps its
 * commands at 10s, its long ones at 30s, and `git fetch` at 60s
 * (internal/node/git). Past that is no answer, not a slow one.
 *
 * It is not a bound on every handler. The long-running lifecycle mutations —
 * create, clone, profile change, delete — pass `timeoutMs: null`, because
 * nothing on the node bounds their slow path, and a ceiling under the real work
 * would tell the user a mutation failed while the node commits it. Rename, pin
 * and the read-marks keep this default: single DB writes, nothing that can
 * outlive a git fetch.
 *
 * Giving up on a read is safe because it leaves no durable state behind, not
 * because the node stops working: several read paths build their context from
 * `context.Background()` (internal/node/git/compare.go, history.go) and the
 * review helper shells out with no context at all
 * (internal/node/git/review/review.go).
 */
export const REQUEST_TIMEOUT_MS = 75_000;

/**
 * Deadline for the polled node reads: DB and tmux reads that return in
 * milliseconds and repeat on a timer. The ceiling above is the wrong shape for
 * them — a request into a black hole would hold the poll loop open long enough
 * that nothing downstream (the sidebar's failure toast, the node's presence
 * dot) can tell you the node stopped answering.
 */
export const POLL_TIMEOUT_MS = 8_000;

export interface ApiRequestInit extends RequestInit {
  /**
   * Overrides REQUEST_TIMEOUT_MS for this request. `null` runs it with no
   * deadline at all.
   *
   * `null` is for the handlers whose slow path nothing bounds — not for
   * handlers that are merely slow. Aborting a `fetch` does not stop the node:
   * it keeps working, so a deadline shorter than the work reports a failure for
   * an operation that goes on to succeed. The real fix is a bound on the
   * server, beside the work.
   *
   * Accepted cost: a request that never settles leaves its mutation pending
   * forever, and the sidebar's busy row and the `isCreating` lock derive from
   * exactly that set (useSessionMutationState), so a wedged node needs a page
   * reload. BXN-133.
   */
  timeoutMs?: number | null;
}

// Node identity is explicit: every fetch/WS helper takes the target node's
// origin (baseUrl) as its first argument. "" == same-origin (the local node).
// Callers obtain it from useActiveNode(); there is no module-level active node.

/**
 * Returns the WebSocket URL for a terminal connection on the node at baseUrl.
 * With sessionId: /api/node/ws/sessions/{id} (attaches to session's tmux)
 * Without: /api/node/ws/terminal (raw shell)
 */
export function nodeWsUrl(baseUrl: string, sessionId?: string | null): string {
  const path = sessionId
    ? `/api/node/ws/sessions/${encodeURIComponent(sessionId)}`
    : "/api/node/ws/terminal";
  if (baseUrl) {
    return baseUrl.replace(/^http/, "ws") + path;
  }
  const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
  return `${protocol}//${window.location.host}${path}`;
}

/**
 * Runs `use` under a deadline that stays armed until the caller has the whole
 * response. `fetch` resolves once the response headers arrive, so a deadline
 * that ends there leaves the body unguarded: a node that sends headers and then
 * stalls mid-JSON hangs as long as one that never answered. Everything that
 * reads the body happens inside this call.
 */
async function withDeadline<T>(
  options: ApiRequestInit | undefined,
  use: (init: RequestInit) => Promise<T>,
): Promise<T> {
  const { timeoutMs = REQUEST_TIMEOUT_MS, signal, ...rest } = options ?? {};

  // No deadline asked for, so none imposed; the caller's own signal still
  // applies. The alternative — a number big enough to look like "never" — still
  // cuts real work on the day something exceeds it.
  if (timeoutMs === null) {
    return use({ ...rest, signal });
  }

  // One controller fed by both the deadline and the caller, rather than
  // `AbortSignal.any`, which would need a fallback on runtimes without it — and
  // any fallback that picks one signal silently drops the other.
  const controller = new AbortController();
  let timedOut = false;
  const timeout = setTimeout(() => {
    // First abort wins: a caller aborting just under the wire leaves the
    // rejection still propagating, and a timer landing in that gap would
    // relabel their cancellation as the node failing to answer. Callers branch
    // on that — useExpandableDiff treats an AbortError as expected and anything
    // else, TimeoutError included, as a transient failure.
    if (controller.signal.aborted) return;
    timedOut = true;
    controller.abort();
  }, timeoutMs);

  if (signal) {
    if (signal.aborted) controller.abort();
    else signal.addEventListener("abort", () => controller.abort(), { once: true });
  }

  try {
    return await use({ ...rest, signal: controller.signal });
  } catch (err) {
    // The DOMException an abort raises says nothing about which abort it was,
    // and both arrive through one controller, so the flag is what tells "the
    // node never answered" from "the caller cancelled".
    if (timedOut) throw new TimeoutError(timeoutMs);
    throw err;
  } finally {
    clearTimeout(timeout);
  }
}

async function baseFetch(
  baseUrl: string,
  url: string,
  init: RequestInit,
): Promise<Response> {
  const res = await fetch(`${baseUrl}${url}`, init);

  if (!res.ok) {
    let message = `Request failed: ${res.status}`;
    try {
      const body = await res.json();
      if (body.error) message = body.error;
    } catch {
      // ignore parse errors
    }
    throw new ApiError(message, res.status);
  }

  return res;
}

export async function apiFetch<T>(
  baseUrl: string,
  url: string,
  options?: ApiRequestInit,
): Promise<T> {
  // Only declare a JSON body when there is one. A Content-Type on a bodyless
  // cross-origin GET makes it a non-"simple" request, forcing a CORS preflight
  // on every read to a remote node (e.g. the status poll) — so set it only for
  // requests that actually carry a body.
  const headers = new Headers(options?.headers);
  if (options?.body != null && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }
  // The parse is inside the deadline, not after it — see withDeadline.
  return withDeadline({ ...options, headers }, async (init) => {
    const res = await baseFetch(baseUrl, url, init);
    return res.json() as Promise<T>;
  });
}

/**
 * Fetch text content from the API. Used for endpoints that send/receive raw
 * text instead of JSON (e.g. file content), and for the ones whose body is
 * empty and discarded (heartbeat, acknowledge, read/unread).
 *
 * Returns the body rather than the `Response`, so the read happens inside the
 * deadline. A live `Response` would put the drain after the timer is cleared,
 * leaving a node that sends headers and then stalls to hang the editor forever.
 * No caller streams.
 */
export async function apiTextFetch(
  baseUrl: string,
  url: string,
  options?: ApiRequestInit,
): Promise<string> {
  return withDeadline(options, async (init) => {
    const res = await baseFetch(baseUrl, url, init);
    return res.text();
  });
}
