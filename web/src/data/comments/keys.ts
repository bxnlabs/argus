export const commentKeys = {
  all: ["comments"] as const,
  forComparison: (path: string, base: string) =>
    [...commentKeys.all, path, base] as const,
};
