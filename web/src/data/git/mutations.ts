import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { apiFetch } from "@/api/client";
import { gitKeys } from "./keys";

interface GitFetchResponse {
  status: string;
}

/**
 * Runs `git fetch --prune` on the backend for the remote that drives the UI's
 * freshness signals (HEAD's upstream remote, falling back to origin), then
 * invalidates cache entries whose values depend on those tracking refs
 * (status ahead/behind, compare-base resolution, branch lists). Surfaces
 * fetch failures (auth, network, git errors) as a toast so the stale-base
 * hint does not silently remain stale.
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
