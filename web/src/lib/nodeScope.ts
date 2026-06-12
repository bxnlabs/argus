/**
 * Cache-identity token for a node, "<id>:<url>". It goes into every node-scoped
 * query key and the TabProvider remount key, so two nodes' data never collide
 * and switching nodes addresses a different cache entry. Including the url means
 * editing a manual node's origin (same id) re-scopes its caches and tabs.
 *
 * This is the single source of the token's shape — useActiveNode (cache keys)
 * and App.tsx (the workspace remount key) must produce byte-identical strings,
 * so both call this rather than rebuilding it inline.
 */
export function nodeScope(id: string | null, url: string): string {
  return `${id ?? "none"}:${url}`;
}
