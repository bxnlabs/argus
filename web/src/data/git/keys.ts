// scope ("<id>:<url>", from useActiveNode) is the first key segment after the
// domain tag so each node's git data is a distinct cache entry. It precedes
// path, so the prefix helpers (comparesByPath, branchesAll) stay correct.
export const gitKeys = {
  all: (scope: string) => ["git", scope] as const,
  check: (scope: string, path: string) =>
    [...gitKeys.all(scope), "check", path] as const,
  status: (scope: string, path: string) =>
    [...gitKeys.all(scope), "status", path] as const,
  history: (scope: string, path: string) =>
    [...gitKeys.all(scope), "history", path] as const,
  commitDetail: (scope: string, path: string, hash: string) =>
    [...gitKeys.all(scope), "commit", path, hash] as const,
  fileDiff: (scope: string, path: string, file: string, staged?: boolean, untracked?: boolean) =>
    [...gitKeys.all(scope), "diff", path, file, staged, untracked] as const,
  fileContent: (scope: string, path: string, file: string) =>
    [...gitKeys.all(scope), "content", path, file] as const,
  compareBranches: (scope: string, path: string) =>
    [...gitKeys.all(scope), "compare-branches", path] as const,
  compare: (scope: string, path: string, base: string) =>
    [...gitKeys.all(scope), "compare", path, base] as const,
  // Prefix matching all compare queries for a path across any base — used to
  // sweep cached compares when the base's upstream tip may have moved.
  comparesByPath: (scope: string, path: string) =>
    [...gitKeys.all(scope), "compare", path] as const,
  commitFullDiff: (scope: string, path: string, hash: string) =>
    [...gitKeys.all(scope), "commit-full-diff", path, hash] as const,
  workingDiff: (scope: string, path: string) =>
    [...gitKeys.all(scope), "working-diff", path] as const,
  branches: (scope: string, source: string) =>
    [...gitKeys.all(scope), "branches", source] as const,
  // Prefix matching every branches query regardless of source.
  branchesAll: (scope: string) => [...gitKeys.all(scope), "branches"] as const,
};
