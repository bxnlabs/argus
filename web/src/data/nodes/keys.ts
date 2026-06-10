// Key prefixes "nodes" and "node-summary" are deliberately distinct from the
// node-scoped keys (sessions, session-statuses, git, …) so NodeProvider can
// drop the latter on switch while keeping the rail's data alive.
export const nodeKeys = {
  all: ["nodes"] as const,
  list: () => [...nodeKeys.all, "list"] as const,
  // url is part of the key because the summary is fetched from the node's own
  // origin: editing a manual node's URL keeps its id but must invalidate the
  // cached summary so the rail/status don't show the old origin's data.
  summary: (id: string, url: string) => ["node-summary", id, url] as const,
};
