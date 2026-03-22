export const commentKeys = {
  all: ["comments"] as const,
  forComparison: (path: string, branch: string, base: string) =>
    [...commentKeys.all, path, branch, base] as const,
};
