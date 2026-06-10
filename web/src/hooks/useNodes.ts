import { useQueries } from "@tanstack/react-query";
import { useNodesQuery } from "@/data/nodes/queries";
import { fetchSummary } from "@/data/nodes/api";
import { nodeKeys } from "@/data/nodes/keys";
import { deriveNodeName } from "@/data/nodes/deriveName";
import type { NodeInfo, NodeWithStatus } from "@/types";

// Synthetic local node used when the registry itself can't be reached. The
// registry is same-origin, so even when its read fails the local node API is
// almost certainly still up; surfacing it (url "" == same-origin) keeps the app
// usable against this machine instead of showing an empty rail.
const LOCAL_FALLBACK: NodeInfo = {
  id: "local",
  name: "This machine",
  url: "",
  source: "local",
  self: true,
};

/**
 * Returns the registered nodes, each enriched with its latest summary and an
 * online flag. Summaries are polled per node every 5s against that node's own
 * origin; a node whose poll errors is marked offline (badges cleared) and keeps
 * retrying so it recovers automatically.
 */
export function useNodes(): { nodes: NodeWithStatus[]; isLoaded: boolean } {
  const { data: registered = [], isSuccess, isError } = useNodesQuery();

  // On a registry error with nothing to show, fall back to the local
  // (same-origin) node so the app stays usable while the registry recovers.
  // (A failed *refetch* keeps the prior data, so registered is only empty on a
  // cold error — exactly when the fallback is needed.)
  const list = isError && registered.length === 0 ? [LOCAL_FALLBACK] : registered;

  const summaries = useQueries({
    queries: list.map((n) => ({
      queryKey: nodeKeys.summary(n.id, n.url),
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
    // erroring". Two failure modes, both covered by isSuccess alone:
    //   1. In-flight retry: a blackholed node keeps retrying, and React Query
    //      clears isError while each (doomed) retry hangs; gating on !isError
    //      would flash the node online for the ~5s before it times out. isSuccess
    //      stays false until a poll actually returns.
    //   2. Failed *background* refetch: with retry:false a settled poll that
    //      errors sets status:'error' (isSuccess false) even though the query
    //      keeps its last data, so a node that succeeded once and then went down
    //      flips offline on the next poll. (Verified against query-core 5.100.14:
    //      a refetch error sets status:'error'; the retained data surfaces as
    //      isRefetchError, not success — so isSuccess alone is sufficient.)
    const online = !!q && q.isSuccess;
    // Pending = the first poll is still in flight (never settled). retry:false
    // means isPending flips false the moment a poll settles and stays false on
    // background refetches, so it marks "Connecting…" without flashing.
    const pending = !!q && q.isPending;
    // Discovered nodes arrive with their raw tailnet hostname (e.g.
    // "argus-bumblebee.tail06de7.ts.net"); run it through the same helper that
    // names manual nodes so the rail, switcher, and panel read consistently.
    // Manual names are user-chosen and local is the machine's own name — leave
    // both untouched.
    const name = n.source === "discovered" ? deriveNodeName(n.name) : n.name;
    return { ...n, name, summary: online && q?.data ? q.data : null, online, pending };
  });

  // "Settled", not merely "succeeded": on a registry error we still surface the
  // local (same-origin) fallback node above, so callers gating on isLoaded must
  // settle (not hang) on error too.
  return { nodes, isLoaded: isSuccess || isError };
}
