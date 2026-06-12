// scope ("<id>:<url>", from useActiveNode) is the first key segment after the
// domain tag so each node's review data is a distinct cache entry.
export const reviewKeys = {
  all: (scope: string) => ["review", scope] as const,
  forComparison: (scope: string, path: string, branch: string, base: string) =>
    [...reviewKeys.all(scope), path, branch, base] as const,
};
