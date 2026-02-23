import { useQuery } from "@tanstack/react-query";
import { apiFetch } from "@/api/client";
import type { FilesResponse, FileSearchResponse, FileMetaResponse } from "@/types";
import { filesKeys } from "./keys";

export function useFilesQuery(
  path: string,
  options?: { enabled?: boolean },
) {
  return useQuery({
    queryKey: filesKeys.list(path),
    queryFn: () =>
      apiFetch<FilesResponse>(`/agent/api/files?path=${encodeURIComponent(path)}`),
    enabled: (options?.enabled ?? true) && path.trim().length > 0,
    staleTime: 10_000,
  });
}

export function useFileSearchQuery(
  query: string,
  options?: { enabled?: boolean; type?: string; limit?: number },
) {
  const type = options?.type ?? "directory";
  const limit = options?.limit ?? 20;

  return useQuery({
    queryKey: filesKeys.search(query, type),
    queryFn: () => {
      const params = new URLSearchParams({
        q: query,
        type,
        limit: String(limit),
      });
      return apiFetch<FileSearchResponse>(`/agent/api/files/search?${params}`);
    },
    enabled: (options?.enabled ?? true) && query.trim().length > 0,
    staleTime: 30_000,
  });
}

export function useFileMetaQuery(
  path: string,
  options?: { enabled?: boolean },
) {
  return useQuery({
    queryKey: filesKeys.meta(path),
    queryFn: () =>
      apiFetch<FileMetaResponse>(
        `/agent/api/files/meta?path=${encodeURIComponent(path)}`,
      ),
    enabled: (options?.enabled ?? true) && path.trim().length > 0,
    staleTime: 30_000,
  });
}
