import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { apiFetch } from "@/api/client";
import { gitKeys } from "./keys";

interface GitFetchResponse {
  status: string;
}

/**
 * Runs `git fetch --prune origin` on the backend for the given working
 * directory, then invalidates cache entries whose values depend on origin/*
 * tracking refs (status ahead/behind, compare-base resolution, branch lists).
 * Surfaces remote-side failures (auth, network, missing remote) as a toast so
 * the stale-base hint does not silently remain stale.
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
    onError: (error) => {
      toast.error(
        `Git fetch failed: ${error instanceof Error ? error.message : String(error)}`,
      );
    },
  });
}
