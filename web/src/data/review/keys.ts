export const reviewKeys = {
  all: ["review"] as const,
  forComparison: (path: string, branch: string, base: string) =>
    [...reviewKeys.all, path, branch, base] as const,
};
