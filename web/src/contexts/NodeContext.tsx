import {
  createContext, useCallback, useContext, useMemo, useState,
} from "react";
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

  // No cache eviction on switch: node-scoped query keys embed the active node's
  // scope (see useActiveNode), so each node's data is a separate cache entry and
  // switching simply addresses different keys. Switching back shows that node's
  // cached data instantly, then React Query refetches in the background.
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
