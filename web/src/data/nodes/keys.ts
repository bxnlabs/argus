// Key prefixes "nodes" and "node-summary" are deliberately distinct from the
// node-scoped keys (sessions, session-statuses, git, …) so NodeProvider can
// drop the latter on switch while keeping the rail's data alive.
export const nodeKeys = {
  all: ["nodes"] as const,
  list: () => [...nodeKeys.all, "list"] as const,
  summary: (id: string) => ["node-summary", id] as const,
};
