import { useQuery } from "@tanstack/react-query";
import { apiFetch } from "@/api/client";
import { useActiveNode } from "@/hooks/useActiveNode";
import type { FilesResponse, FileSearchResponse, FileMetaResponse } from "@/types";
import { filesKeys } from "./keys";

export function useFilesQuery(
  path: string,
  options?: { enabled?: boolean },
) {
  const { scope, baseUrl } = useActiveNode();
  return useQuery({
    queryKey: filesKeys.list(scope, path),
    queryFn: () =>
      apiFetch<FilesResponse>(baseUrl, `/api/node/files?path=${encodeURIComponent(path)}`),
    enabled: (options?.enabled ?? true) && path.trim().length > 0,
    staleTime: 10_000,
  });
}

export function useFileSearchQuery(
  query: string,
  options?: { enabled?: boolean; type?: string; limit?: number; searchPath?: string },
) {
  const { scope, baseUrl } = useActiveNode();
  const type = options?.type ?? "directory";
  const limit = options?.limit ?? 20;
  const searchPath = options?.searchPath;

  return useQuery({
    queryKey: filesKeys.search(scope, query, type, searchPath),
    queryFn: () => {
      const params = new URLSearchParams({
        q: query,
        type,
        limit: String(limit),
      });
      if (searchPath) {
        params.set("path", searchPath);
      }
      return apiFetch<FileSearchResponse>(baseUrl, `/api/node/files/search?${params}`);
    },
    enabled: (options?.enabled ?? true) && query.trim().length > 0,
    staleTime: 30_000,
  });
}

export function useFileMetaQuery(
  path: string,
  options?: { enabled?: boolean },
) {
  const { scope, baseUrl } = useActiveNode();
  return useQuery({
    queryKey: filesKeys.meta(scope, path),
    queryFn: () =>
      apiFetch<FileMetaResponse>(
        baseUrl,
        `/api/node/files/meta?path=${encodeURIComponent(path)}`,
      ),
    enabled: (options?.enabled ?? true) && path.trim().length > 0,
    staleTime: 30_000,
  });
}
