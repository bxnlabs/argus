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
 * Default request deadline.
 *
 * Sized off what the node itself will do rather than picked by feel: the git
 * layer bounds its own work at 10s for most commands, 30s for the long ones,
 * and 60s for `git fetch`, which is the slowest thing any handler will sit on
 * (internal/node/git). Anything past that is not a slow answer, it's no answer
 * — so the ceiling clears the server's own maximum with margin, and the client
 * never cancels work the node would have finished.
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

async function baseFetch(
  baseUrl: string,
  url: string,
  options?: ApiRequestInit,
): Promise<Response> {
  const { timeoutMs = REQUEST_TIMEOUT_MS, signal, ...rest } = options ?? {};

  // Composed rather than replacing the caller's signal, so a deadline never
  // costs an abort the caller was relying on. `AbortSignal.any` is the only
  // piece here that isn't ancient; it's in every browser this app already
  // requires for `structuredClone`-era APIs, and the fallback below keeps the
  // deadline working anywhere it's missing rather than throwing.
  const timer = new AbortController();
  const timeout = setTimeout(() => timer.abort(new TimeoutError(timeoutMs)), timeoutMs);
  const composed =
    signal && typeof AbortSignal.any === "function"
      ? AbortSignal.any([signal, timer.signal])
      : (signal ?? timer.signal);

  let res: Response;
  try {
    res = await fetch(`${baseUrl}${url}`, { ...rest, signal: composed });
  } catch (err) {
    // The DOMException an abort raises says nothing about which abort it was.
    // Re-throw the deadline as itself so callers can tell "the node never
    // answered" from "the user navigated away".
    if (timer.signal.aborted) throw new TimeoutError(timeoutMs);
    throw err;
  } finally {
    clearTimeout(timeout);
  }

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
  const res = await baseFetch(baseUrl, url, { ...options, headers });
  return res.json();
}

/**
 * Fetch text content from the API. Used for endpoints that
 * send/receive raw text instead of JSON (e.g., file content).
 */
export async function apiTextFetch(
  baseUrl: string,
  url: string,
  options?: ApiRequestInit,
): Promise<Response> {
  return baseFetch(baseUrl, url, options);
}
