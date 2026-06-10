import { useNodeContext } from "@/contexts/NodeContext";

export interface ActiveNode {
  // Cache-identity token for the active node, "<id>:<url>". Goes into every
  // node-scoped query key so two nodes' data never collide and switching nodes
  // simply addresses a different cache entry — no global, no eviction needed.
  // Includes the url so editing a manual node's origin (same id) re-scopes.
  scope: string;
  // The active node's origin for fetch/WS calls. "" == same-origin (local).
  baseUrl: string;
}

/**
 * Single source of truth binding API calls and query keys to the active node.
 * The scope mirrors the TabProvider remount key in App.tsx, so the workspace
 * and its caches share one identity.
 */
export function useActiveNode(): ActiveNode {
  const { activeNodeId, activeNode } = useNodeContext();
  const baseUrl = activeNode?.url ?? "";
  return { scope: `${activeNodeId ?? "none"}:${baseUrl}`, baseUrl };
}
