import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "@/api/client";
import { useActiveNode } from "@/hooks/useActiveNode";
import type { Review } from "@/types";
import { reviewKeys } from "./keys";

export function useReviewQuery(
  path: string,
  head: string | undefined,
  base: string | null,
  headRef?: string,
  baseRef?: string,
) {
  const { scope, baseUrl } = useActiveNode();
  return useQuery({
    queryKey: [...reviewKeys.forComparison(scope, path, head ?? "", base ?? ""), headRef ?? "", baseRef ?? ""],
    queryFn: async () => {
      const params = new URLSearchParams({
        path,
        branch: head!,
        base: base!,
      });
      if (headRef) params.set("headRef", headRef);
      if (baseRef) params.set("baseRef", baseRef);
      return apiFetch<Review>(baseUrl, `/api/node/git/review?${params}`);
    },
    staleTime: 30_000,
    enabled: path.trim().length > 0 && !!head && !!base,
  });
}

export function useSaveReviewMutation(path: string) {
  const queryClient = useQueryClient();
  const { scope, baseUrl } = useActiveNode();

  return useMutation({
    mutationFn: async (data: Review) => {
      const params = new URLSearchParams({ path });
      return apiFetch<{ status: string }>(baseUrl, `/api/node/git/review?${params}`, {
        method: "POST",
        body: JSON.stringify(data),
      });
    },
    onSuccess: (_result, data) => {
      // Invalidate the (path, head, base) prefix so any cached review query —
      // regardless of the headRef/baseRef suffix — is refetched after save.
      queryClient.invalidateQueries({
        queryKey: reviewKeys.forComparison(scope, path, data.head, data.base),
      });
    },
  });
}

export function useDeleteReviewMutation(path: string) {
  const queryClient = useQueryClient();
  const { scope, baseUrl } = useActiveNode();

  return useMutation({
    mutationFn: async ({
      head,
      base,
    }: {
      head: string;
      base: string;
    }) => {
      const params = new URLSearchParams({
        path,
        branch: head,
        base,
      });
      return apiFetch<{ status: string }>(
        baseUrl,
        `/api/node/git/review?${params}`,
        { method: "DELETE" },
      );
    },
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: reviewKeys.forComparison(scope, path, variables.head, variables.base),
      });
    },
  });
}
