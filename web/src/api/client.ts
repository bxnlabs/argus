export class ApiError extends Error {
  constructor(
    message: string,
    public status: number,
  ) {
    super(message);
    this.name = "ApiError";
  }
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
  options?: RequestInit,
): Promise<Response> {
  const res = await fetch(`${baseUrl}${url}`, options);

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
  options?: RequestInit,
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
  options?: RequestInit,
): Promise<Response> {
  return baseFetch(baseUrl, url, options);
}
