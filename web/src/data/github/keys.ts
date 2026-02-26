export const githubKeys = {
  all: ["github"] as const,
  repos: (query: string) => [...githubKeys.all, "repos", query] as const,
};
