export const filesKeys = {
  all: ["files"] as const,
  list: (path: string) => [...filesKeys.all, "list", path] as const,
  search: (query: string, type?: string) =>
    [...filesKeys.all, "search", query, type] as const,
  meta: (path: string) => [...filesKeys.all, "meta", path] as const,
  content: (path: string) => [...filesKeys.all, "content", path] as const,
};
