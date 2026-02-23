export class ApiError extends Error {
  constructor(
    message: string,
    public status: number,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

/**
 * Returns the base URL for the agent API.
 * - In dev mode: empty string (Vite proxy handles forwarding)
 * - In production: reads VITE_AGENT_URL, defaults to same origin
 */
export function getAgentBaseUrl(): string {
  return import.meta.env.VITE_AGENT_URL || "";
}

/**
 * Returns the WebSocket URL for a terminal connection.
 * With sessionId: /agent/ws/sessions/{id} (attaches to session's tmux)
 * Without: /agent/ws/terminal (raw shell)
 */
export function getAgentWsUrl(sessionId?: string | null): string {
  const base = getAgentBaseUrl();
  const path = sessionId
    ? `/agent/ws/sessions/${encodeURIComponent(sessionId)}`
    : "/agent/ws/terminal";
  if (base) {
    return base.replace(/^http/, "ws") + path;
  }
  const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
  return `${protocol}//${window.location.host}${path}`;
}

async function baseFetch(
  url: string,
  options?: RequestInit,
): Promise<Response> {
  const base = getAgentBaseUrl();
  const res = await fetch(`${base}${url}`, options);

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
  url: string,
  options?: RequestInit,
): Promise<T> {
  const res = await baseFetch(url, {
    ...options,
    headers: {
      "Content-Type": "application/json",
      ...options?.headers,
    },
  });
  return res.json();
}

/**
 * Fetch text content from the API. Used for endpoints that
 * send/receive raw text instead of JSON (e.g., file content).
 */
export async function apiTextFetch(
  url: string,
  options?: RequestInit,
): Promise<Response> {
  return baseFetch(url, options);
}
