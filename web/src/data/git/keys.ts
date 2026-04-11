export const gitKeys = {
  all: ["git"] as const,
  check: (path: string) => [...gitKeys.all, "check", path] as const,
  status: (path: string) => [...gitKeys.all, "status", path] as const,
  history: (path: string) => [...gitKeys.all, "history", path] as const,
  commitDetail: (path: string, hash: string) =>
    [...gitKeys.all, "commit", path, hash] as const,
  fileDiff: (path: string, file: string, staged?: boolean, untracked?: boolean) =>
    [...gitKeys.all, "diff", path, file, staged, untracked] as const,
  fileContent: (path: string, file: string) =>
    [...gitKeys.all, "content", path, file] as const,
  compareBranches: (path: string) =>
    [...gitKeys.all, "compare-branches", path] as const,
  compare: (path: string, base: string) =>
    [...gitKeys.all, "compare", path, base] as const,
  commitFullDiff: (path: string, hash: string) =>
    [...gitKeys.all, "commit-full-diff", path, hash] as const,
  workingDiff: (path: string) => [...gitKeys.all, "working-diff", path] as const,
  branches: (source: string) =>
    [...gitKeys.all, "branches", source] as const,
};
