import type { ReactNode } from "react";
import { NodeContext } from "@/contexts/NodeContext";
import type { NodeWithStatus } from "@/types";

const localNode: NodeWithStatus = {
  id: "local",
  name: "this",
  url: "",
  source: "local",
  self: true,
  summary: null,
  online: true,
  pending: false,
};

/**
 * Wraps children in a NodeContext fixed to the local node (scope "local:",
 * same-origin baseUrl). For unit tests of components/hooks that call
 * useActiveNode but don't exercise node switching.
 */
export function StubNodeProvider({ children }: { children: ReactNode }) {
  return (
    <NodeContext.Provider
      value={{
        nodes: [localNode],
        isLoaded: true,
        activeNodeId: "local",
        activeNode: localNode,
        setActiveNode: () => {},
      }}
    >
      {children}
    </NodeContext.Provider>
  );
}
