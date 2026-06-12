import type { NodeInfo, NodeSummary } from "@/types";

// Registry is served by the instance that served the SPA (same-origin),
// regardless of which node is active — so these bypass the node base URL.
export async function fetchNodes(): Promise<NodeInfo[]> {
  const res = await fetch("/api/nodes");
  if (!res.ok) throw new Error(`fetch nodes: ${res.status}`);
  const body = (await res.json()) as { nodes?: NodeInfo[] };
  return body.nodes ?? [];
}

async function mutate(method: string, path: string, body?: unknown): Promise<void> {
  const res = await fetch(path, {
    method,
    headers: { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (!res.ok) {
    const detail = (await res.json().catch(() => ({}))) as { error?: string };
    throw new Error(detail.error ?? `${method} ${path}: ${res.status}`);
  }
}

export const addNode = (name: string, url: string) =>
  mutate("POST", "/api/nodes", { name, url });
export const updateNode = (id: string, name: string, url: string) =>
  mutate("PATCH", `/api/nodes/${encodeURIComponent(id)}`, { name, url });
export const deleteNode = (id: string) =>
  mutate("DELETE", `/api/nodes/${encodeURIComponent(id)}`);

// A summary fetch must fail fast: a blackholed node (dropped packets, not a
// refused connection) leaves fetch pending until the OS TCP timeout (tens of
// seconds), during which the node wrongly shows online. Cap it so the poll
// errors and the rail flips the node offline promptly.
const SUMMARY_TIMEOUT_MS = 5_000;

// Summary is fetched against each node's own origin (cross-origin for remote
// nodes; "" == same-origin for the local node). Aborts on the caller's signal
// (React Query cancellation) or the timeout, whichever fires first.
export async function fetchSummary(baseUrl: string, signal?: AbortSignal): Promise<NodeSummary> {
  const timeout = AbortSignal.timeout(SUMMARY_TIMEOUT_MS);
  const res = await fetch(`${baseUrl}/api/node/summary`, {
    signal: signal ? AbortSignal.any([signal, timeout]) : timeout,
  });
  if (!res.ok) throw new Error(`summary: ${res.status}`);
  return (await res.json()) as NodeSummary;
}
