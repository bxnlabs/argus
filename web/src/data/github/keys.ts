// scope ("<id>:<url>", from useActiveNode) is the first key segment after the
// domain tag so each node's GitHub data is a distinct cache entry.
export const githubKeys = {
  all: (scope: string) => ["github", scope] as const,
  repos: (scope: string, query: string) =>
    [...githubKeys.all(scope), "repos", query] as const,
};
