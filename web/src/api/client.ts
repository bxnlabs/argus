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
 * Clears the slowest bound the node puts on that kind of work: the git layer
 * caps its commands at 10s, its long ones at 30s, and `git fetch` at 60s
 * (internal/node/git). Past that is not a slow answer, it's no answer.
 *
 * It is emphatically not a bound on every handler — session create, clone and
 * profile change can sit on a `docker compose up` the node allows 20 minutes
 * for, and those opt into SESSION_OPERATION_TIMEOUT_MS below. Anything added
 * here that can outlive a git fetch needs the same treatment; a default that
 * cuts real work is worse than no default at all, because it reports a failure
 * for an operation that is still running and will succeed.
 */
export const REQUEST_TIMEOUT_MS = 75_000;

/**
 * Deadline for the session operations that can bring a Docker stack up.
 *
 * Mirrors the CLI's `profileStackClientTimeout` (cmd/argus/cli/client.go) and
 * for its reason: it sits just above the node's own `stackOpTimeout` of 20
 * minutes (internal/node/session/docker.go) so the server's bound stays the
 * source of truth for the compose operation, while still capping the wait if a
 * handler wedges somewhere outside that bounded path.
 *
 * Create can also run an unbounded `git clone` for a remote source
 * (internal/git/worktree) — nothing on either side bounds that today, so a
 * clone slower than this would still be cut off. The CLI carries the same
 * exposure; bounding it belongs on the server, next to the clone.
 */
export const SESSION_OPERATION_TIMEOUT_MS = 25 * 60_000;

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
  /** Overrides REQUEST_TIMEOUT_MS for this request. */
  timeoutMs?: number;
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

  // One controller fed by both the deadline and the caller, rather than
  // `AbortSignal.any`. Composing by hand is barely longer and it cannot fail
  // open: `any` would need a fallback for runtimes without it, and every
  // fallback that picks one signal over the other silently drops whichever it
  // didn't pick — a deadline that quietly stops existing on some browsers is
  // worse than one that was never claimed.
  const controller = new AbortController();
  let timedOut = false;
  const timeout = setTimeout(() => {
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
 * Fetch text content from the API. Used for endpoints that
 * send/receive raw text instead of JSON (e.g., file content).
 *
 * The deadline covers reaching the response, not draining it: the body belongs
 * to the caller here, so it is read after this returns and the timer is already
 * cleared. Callers that stream something large are the ones this shape suits;
 * a caller wanting the body bounded too should read it through `apiFetch`.
 */
export async function apiTextFetch(
  baseUrl: string,
  url: string,
  options?: ApiRequestInit,
): Promise<Response> {
  return withDeadline(options, (init) => baseFetch(baseUrl, url, init));
}
