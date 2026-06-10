import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { apiFetch } from "@/api/client";
import { useActiveNode } from "@/hooks/useActiveNode";
import { gitKeys } from "./keys";

interface FetchVars {
  path: string;
  /**
   * Optional compare base. When provided, the backend additionally refreshes
   * the remote that the base branch's upstream lives on — required for fork
   * workflows where HEAD and the compare base track different remotes (e.g.
   * HEAD → origin/feature, main → upstream/main). Without this, the compare
   * stale-base banner can't be cleared by clicking refresh.
   */
  base?: string | null;
}

/**
 * Runs `git fetch --prune` on the backend for the remotes whose tracking refs
 * the UI's freshness signals depend on (HEAD's upstream and, when supplied,
 * the compare base's upstream), then invalidates cache entries derived from
 * those refs (status ahead/behind, compare-base resolution, branch lists).
 * Surfaces fetch failures (auth, network, git errors) as a toast so the
 * stale-base hint does not silently remain stale.
 *
 * Invalidations fire on settle, not just success, so:
 *   - a partial-success fetch (one remote up, one down) still propagates the
 *     successful remote's data into the UI rather than getting stranded by
 *     the wrapped error from the failed remote, and
 *   - local-state changes (e.g. the user committed locally between cache
 *     write and refresh click) flow into the compare view even when the
 *     network fetch itself failed — compare depends on local HEAD/base refs,
 *     not just remote ones.
 */
export function useGitFetchMutation() {
  const queryClient = useQueryClient();
  const { scope, baseUrl } = useActiveNode();

  return useMutation({
    mutationFn: async ({ path, base }: FetchVars) => {
      const params = new URLSearchParams({ path });
      if (base) {
        params.set("base", base);
      }
      return apiFetch<{ status: string }>(
        baseUrl,
        `/api/node/git/fetch?${params.toString()}`,
        { method: "POST" },
      );
    },
    onSettled: (_data, _error, vars) => {
      if (!vars) return;
      const { path } = vars;
      queryClient.invalidateQueries({ queryKey: gitKeys.status(scope, path) });
      queryClient.invalidateQueries({ queryKey: gitKeys.compareBranches(scope, path) });
      queryClient.invalidateQueries({ queryKey: gitKeys.comparesByPath(scope, path) });
      queryClient.invalidateQueries({ queryKey: gitKeys.branchesAll(scope) });
    },
    onError: (error) => {
      toast.error(
        `Git fetch failed: ${error instanceof Error ? error.message : String(error)}`,
      );
    },
  });
}
