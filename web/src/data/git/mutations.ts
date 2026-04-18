import { useMutation, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "@/api/client";
import { gitKeys } from "./keys";

interface GitFetchResponse {
  status: string;
}

/**
 * Runs `git fetch --prune` on the backend for the given working directory,
 * then invalidates cache entries whose values depend on origin/* tracking
 * refs (status ahead/behind, compare-base resolution, branch lists).
 */
export function useGitFetchMutation() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (path: string) => {
      return apiFetch<GitFetchResponse>(
        `/node/api/git/fetch?path=${encodeURIComponent(path)}`,
        { method: "POST" },
      );
    },
    onSuccess: (_data, path) => {
      queryClient.invalidateQueries({ queryKey: gitKeys.status(path) });
      queryClient.invalidateQueries({ queryKey: gitKeys.compareBranches(path) });
      queryClient.invalidateQueries({
        queryKey: [...gitKeys.all, "compare", path],
      });
      queryClient.invalidateQueries({ queryKey: [...gitKeys.all, "branches"] });
    },
  });
}
