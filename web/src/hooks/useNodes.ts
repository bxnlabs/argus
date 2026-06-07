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
  const { data: list = [], isSuccess } = useNodesQuery();

  const summaries = useQueries({
    queries: list.map((n) => ({
      queryKey: nodeKeys.summary(n.id),
      queryFn: () => fetchSummary(n.url),
      refetchInterval: 5_000,
      retry: false,
      // Keep polling a downed node so it recovers without a manual refresh.
      refetchIntervalInBackground: true,
    })),
  });

  const nodes: NodeWithStatus[] = list.map((n, i) => {
    const q = summaries[i];
    const online = !!q && !q.isError;
    return { ...n, summary: online && q?.data ? q.data : null, online };
  });

  return { nodes, isLoaded: isSuccess };
}
