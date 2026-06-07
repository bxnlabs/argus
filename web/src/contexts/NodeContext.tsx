import {
  createContext, useCallback, useContext, useLayoutEffect, useMemo, useRef, useState,
} from "react";
import { useQueryClient, type QueryClient } from "@tanstack/react-query";
import { useNodes } from "@/hooks/useNodes";
import { setActiveNodeBaseUrl } from "@/api/client";
import type { NodeWithStatus } from "@/types";

const STORAGE_KEY = "argus.activeNodeId";

// Drop every node-scoped cache (sessions/statuses/git/files/profiles/…) so the
// newly-active node loads fresh; keep the registry + summaries (keys "nodes"/
// "node-summary") that feed the rail.
function dropNodeScopedQueries(queryClient: QueryClient): void {
  queryClient.removeQueries({
    predicate: (q) => {
      const k = q.queryKey[0];
      return k !== "nodes" && k !== "node-summary";
    },
  });
}

interface NodeContextValue {
  nodes: NodeWithStatus[];
  isLoaded: boolean;
  activeNodeId: string | null;
  activeNode: NodeWithStatus | null;
  setActiveNode: (id: string) => void;
}

const NodeContext = createContext<NodeContextValue | null>(null);

export function NodeProvider({ children }: { children: React.ReactNode }) {
  const queryClient = useQueryClient();
  const { nodes, isLoaded } = useNodes();
  const [activeNodeId, setActiveNodeId] = useState<string | null>(
    () => localStorage.getItem(STORAGE_KEY),
  );

  // Resolve the selection; default to the local/self node, else the first node.
  const activeNode = useMemo(() => {
    if (nodes.length === 0) return null;
    return (
      nodes.find((n) => n.id === activeNodeId) ??
      nodes.find((n) => n.self) ??
      nodes[0]
    );
  }, [nodes, activeNodeId]);

  // The active origin as of the last time we applied it / evicted caches. Used
  // to coordinate the two eviction paths below so they never double-fire:
  // `setActiveNode` evicts synchronously (before the remount renders, so no
  // stale read), and the layout effect catches *implicit* origin changes the
  // explicit handler never sees — first load with a persisted remote node, and
  // deletion of the active node (both resolve a new `activeNode` via the memo).
  // `null` means nothing has been applied yet (first run → nothing to evict).
  const appliedOriginRef = useRef<string | null>(null);

  // Apply the active origin before child queries fetch (passive effects run
  // after this layout effect, so the first session fetch sees the right base),
  // and evict the previous node's caches when the origin changed implicitly.
  useLayoutEffect(() => {
    const origin = activeNode?.url ?? "";
    setActiveNodeBaseUrl(origin);
    if (appliedOriginRef.current !== null && appliedOriginRef.current !== origin) {
      // An implicit switch (setActiveNode keeps appliedOriginRef in sync, so it
      // skips this) — drop the old node's data so it never flashes into the new.
      dropNodeScopedQueries(queryClient);
    }
    appliedOriginRef.current = origin;
  }, [activeNode?.url, queryClient]);

  const setActiveNode = useCallback(
    (id: string) => {
      const target = nodes.find((n) => n.id === id);
      if (!target || id === activeNode?.id) return;
      setActiveNodeBaseUrl(target.url);
      localStorage.setItem(STORAGE_KEY, id);
      dropNodeScopedQueries(queryClient);
      // Keep the layout effect from evicting again on the resulting re-render.
      appliedOriginRef.current = target.url;
      setActiveNodeId(id);
    },
    [nodes, activeNode?.id, queryClient],
  );

  const value = useMemo<NodeContextValue>(
    () => ({ nodes, isLoaded, activeNodeId: activeNode?.id ?? null, activeNode, setActiveNode }),
    [nodes, isLoaded, activeNode, setActiveNode],
  );

  return <NodeContext.Provider value={value}>{children}</NodeContext.Provider>;
}

export function useNodeContext(): NodeContextValue {
  const ctx = useContext(NodeContext);
  if (!ctx) throw new Error("useNodeContext must be used within NodeProvider");
  return ctx;
}

export { NodeContext };
