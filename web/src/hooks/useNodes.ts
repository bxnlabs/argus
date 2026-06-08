import { useQueries } from "@tanstack/react-query";
import { useNodesQuery } from "@/data/nodes/queries";
import { fetchSummary } from "@/data/nodes/api";
import { nodeKeys } from "@/data/nodes/keys";
import type { NodeWithStatus } from "@/types";

/**
 * Returns the registered nodes, each enriched with its latest summary and an
 * online flag. Summaries are polled per node every 5s against that node's own
 * origin; a node whose poll errors is marked offline (badges cleared) and keeps
 * retrying so it recovers automatically.
 */
export function useNodes(): { nodes: NodeWithStatus[]; isLoaded: boolean } {
  const { data: list = [], isSuccess, isError } = useNodesQuery();

  const summaries = useQueries({
    queries: list.map((n) => ({
      queryKey: nodeKeys.summary(n.id),
      queryFn: ({ signal }) => fetchSummary(n.url, signal),
      refetchInterval: 5_000,
      retry: false,
      // Keep polling a downed node so it recovers without a manual refresh.
      refetchIntervalInBackground: true,
    })),
  });

  const nodes: NodeWithStatus[] = list.map((n, i) => {
    const q = summaries[i];
    // Online means the *last settled* poll succeeded — not merely "not currently
    // erroring". A blackholed node keeps retrying, and React Query clears isError
    // while each (doomed) retry is in flight; gating on !isError would flash the
    // node online for the ~5s the retry hangs before it times out. isSuccess only
    // holds once a poll actually returned, so a downed node stays steadily offline
    // and a healthy node stays online across background refetches.
    const online = !!q && q.isSuccess;
    // Pending = the first poll is still in flight (never settled). retry:false
    // means isPending flips false the moment a poll settles and stays false on
    // background refetches, so it marks "Connecting…" without flashing.
    const pending = !!q && q.isPending;
    return { ...n, summary: online && q?.data ? q.data : null, online, pending };
  });

  // "Settled", not merely "succeeded": on a registry error we fall back to the
  // local (same-origin) node, so callers gating on isLoaded must not hang.
  return { nodes, isLoaded: isSuccess || isError };
}
