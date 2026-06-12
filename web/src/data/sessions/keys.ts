// scope ("<id>:<url>", from useActiveNode) is the first key segment so each
// node's data is a distinct cache entry; switching nodes addresses a different
// key rather than relying on eviction.
export const sessionKeys = {
  all: (scope: string) => ["sessions", scope] as const,
  list: (scope: string) => [...sessionKeys.all(scope), "list"] as const,
};

export const statusKeys = {
  all: (scope: string) => ["session-statuses", scope] as const,
};

export const profileKeys = {
  all: (scope: string) => ["profiles", scope] as const,
  list: (scope: string) => [...profileKeys.all(scope), "list"] as const,
};
