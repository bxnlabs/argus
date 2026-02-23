export const gitKeys = {
  all: ["git"] as const,
  check: (path: string) => [...gitKeys.all, "check", path] as const,
  status: (path: string) => [...gitKeys.all, "status", path] as const,
  history: (path: string) => [...gitKeys.all, "history", path] as const,
  commitDetail: (path: string, hash: string) =>
    [...gitKeys.all, "commit", path, hash] as const,
  fileDiff: (path: string, file: string, staged?: boolean, untracked?: boolean) =>
    [...gitKeys.all, "diff", path, file, staged, untracked] as const,
  commitFileDiff: (path: string, hash: string, file: string) =>
    [...gitKeys.all, "commit-diff", path, hash, file] as const,
  fileContent: (path: string, file: string) =>
    [...gitKeys.all, "content", path, file] as const,
};
