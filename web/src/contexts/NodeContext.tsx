import {
  createContext, useCallback, useContext, useLayoutEffect, useMemo, useState,
} from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useNodes } from "@/hooks/useNodes";
import { setActiveNodeBaseUrl } from "@/api/client";
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

  // Apply the active origin before child queries fetch (passive effects run
  // after this layout effect, so the first session fetch sees the right base).
  useLayoutEffect(() => {
    setActiveNodeBaseUrl(activeNode?.url ?? "");
  }, [activeNode?.url]);

  const setActiveNode = useCallback(
    (id: string) => {
      const target = nodes.find((n) => n.id === id);
      if (!target || id === activeNode?.id) return;
      setActiveNodeBaseUrl(target.url);
      localStorage.setItem(STORAGE_KEY, id);
      // Drop node-scoped caches (sessions/statuses/git/files/profiles/…) so the
      // new node loads fresh; keep the registry + summaries that feed the rail.
      queryClient.removeQueries({
        predicate: (q) => {
          const k = q.queryKey[0];
          return k !== "nodes" && k !== "node-summary";
        },
      });
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
