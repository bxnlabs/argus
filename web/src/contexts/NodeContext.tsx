import {
  createContext, useCallback, useContext, useEffect, useMemo, useState,
} from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useNodes } from "@/hooks/useNodes";
import type { NodeWithStatus } from "@/types";

const STORAGE_KEY = "argus.activeNodeId";

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

  // The active node's cache scope ("<id>:<url>"), identical to useActiveNode and
  // the TabProvider remount key. Node-scoped query keys embed this, so each
  // node's data is a separate cache entry and switching is correct *without* any
  // eviction — the new node's queries simply address different keys. We still GC
  // the previous node's entries here purely to bound memory and preserve the
  // fresh-load-on-switch UX; correctness does not depend on it. Drop to instant-
  // switch-with-cache later by removing this effect.
  const scope = `${activeNode?.id ?? "none"}:${activeNode?.url ?? ""}`;
  useEffect(() => {
    queryClient.removeQueries({
      predicate: (q) => {
        const tag = q.queryKey[0];
        // The rail's registry + per-node summaries are global; keep them.
        if (tag === "nodes" || tag === "node-summary") return false;
        // Any other-node scoped entry (different scope at index 1) is dropped.
        return q.queryKey[1] !== scope;
      },
    });
  }, [scope, queryClient]);

  const setActiveNode = useCallback(
    (id: string) => {
      const target = nodes.find((n) => n.id === id);
      if (!target || id === activeNode?.id) return;
      localStorage.setItem(STORAGE_KEY, id);
      setActiveNodeId(id);
    },
    [nodes, activeNode?.id],
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
