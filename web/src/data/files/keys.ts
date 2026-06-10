// scope ("<id>:<url>", from useActiveNode) is the first key segment after the
// domain tag so each node's file data is a distinct cache entry.
export const filesKeys = {
  all: (scope: string) => ["files", scope] as const,
  list: (scope: string, path: string) =>
    [...filesKeys.all(scope), "list", path] as const,
  search: (scope: string, query: string, type?: string, searchPath?: string) =>
    [...filesKeys.all(scope), "search", query, type, searchPath] as const,
  meta: (scope: string, path: string) =>
    [...filesKeys.all(scope), "meta", path] as const,
  content: (scope: string, path: string) =>
    [...filesKeys.all(scope), "content", path] as const,
};
