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
 * Default request deadline, for the ordinary request that reads something.
 *
 * Clears the slowest bound most of that work carries: the git layer caps its
 * commands at 10s, its long ones at 30s, and `git fetch` at 60s
 * (internal/node/git). Past that is not a slow answer, it's no answer.
 *
 * It is not a bound on every handler. The long-running lifecycle mutations —
 * create, clone, profile change, delete — reach work nothing on the node
 * bounds: an unbounded `git clone`, compose calls bounded only one at a time,
 * worktree and branch operations through a context-less `git.Run`, and session
 * and profile lock waits that sit outside every context there is. They pass
 * `timeoutMs: null`, because a ceiling below the real work tells the user a
 * mutation failed while the node commits it. Rename, pin and the read-marks
 * keep this default: they are single DB writes, and nothing about them outlives
 * a git fetch. Anything added that can needs the same treatment.
 *
 * What keeps the default honest on reads is not that the node stops working —
 * it frequently doesn't. Several read paths build their context from
 * `context.Background()` (internal/node/git/compare.go, history.go) and the
 * review helper shells out with no context at all
 * (internal/node/git/review/review.go), so an abandoned read runs to its own
 * end regardless. What makes giving up safe is that it leaves nothing behind:
 * no durable state that can later contradict the failure we reported, which is
 * exactly what an abandoned mutation does leave.
 */
export const REQUEST_TIMEOUT_MS = 75_000;

/**
 * Deadline for the polled node reads.
 *
 * These are DB and tmux reads that return in milliseconds, and they repeat on a
 * timer, so the generous ceiling above is the wrong shape for them: a request
 * into a black hole would hold the poll loop open long enough that nothing
 * downstream — the sidebar's failure toast, the node's presence dot — can tell
 * you the node stopped answering. Short enough to notice, long enough to
 * survive a slow link.
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
   * an operation that goes on to succeed, and the user retries a create that
   * already happened. Where no honest ceiling exists, no ceiling is the safer
   * answer, and the real fix is a bound on the server beside the work.
   *
   * Known cost, accepted rather than overlooked: a request that never settles
   * leaves its mutation pending forever, and the sidebar's busy row and the
   * `isCreating` lock are derived from exactly that set
   * (useSessionMutationState). So a wedged node leaves a row inert, or new
   * sessions refused, until the page is reloaded — the deadlines used to
   * release those. Reload is the recovery; there is deliberately no
   * "stop waiting" control, because the honest fix is a server-side bound and
   * a local release would only hide a node that is still stuck. BXN-133.
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
 * Runs `use` under a deadline, and — the part that matters — keeps the deadline
 * running for as long as the caller needs the response.
 *
 * `fetch` resolves once the response *headers* arrive, so a deadline that ends
 * there leaves the body unguarded: a node that sends headers and then stalls
 * mid-JSON hangs exactly as long as one that never answered at all. Everything
 * that reads the body therefore happens inside this call, not after it.
 */
async function withDeadline<T>(
  options: ApiRequestInit | undefined,
  use: (init: RequestInit) => Promise<T>,
): Promise<T> {
  const { timeoutMs = REQUEST_TIMEOUT_MS, signal, ...rest } = options ?? {};

  // No deadline asked for, so none imposed — the caller's own signal still
  // applies. Opting out has to be expressible: the alternative is a number
  // large enough to look like "never", which is a deadline that cuts real work
  // on the day something exceeds it, and does it under a name that says it
  // won't.
  if (timeoutMs === null) {
    return use({ ...rest, signal });
  }

  // One controller fed by both the deadline and the caller, rather than
  // `AbortSignal.any`. Composing by hand is barely longer and it cannot fail
  // open: `any` would need a fallback for runtimes without it, and every
  // fallback that picks one signal over the other silently drops whichever it
  // didn't pick — a deadline that quietly stops existing on some browsers is
  // worse than one that was never claimed.
  const controller = new AbortController();
  let timedOut = false;
  const timeout = setTimeout(() => {
    // First abort wins. Without this the flag records only *that* the timer
    // fired, not that it fired first: a caller aborting just under the wire
    // leaves the rejection propagating through a microtask, and a timer landing
    // in that gap relabels their cancellation as the node failing to answer.
    // Callers do act on the difference — useExpandableDiff treats an AbortError
    // as an expected cancellation and anything else, TimeoutError included, as
    // a transient failure worth recording against the anchor.
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
    // and now that both aborts arrive through one controller, the signal can't
    // either — hence the flag. Callers need to tell "the node never answered",
    // which the sidebar announces, from "the caller cancelled", which it must
    // not.
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
 * empty and simply discarded (heartbeat, acknowledge, read/unread).
 *
 * Returns the body rather than the `Response`, so the read happens inside the
 * deadline — same reason `apiFetch` parses inside it. Handing back a live
 * `Response` puts the drain after the timer is cleared, which left a node that
 * sends headers and then stalls hanging the editor forever: the one failure a
 * deadline is supposed to cover. No caller streams, so there is nothing the
 * `Response` bought.
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
