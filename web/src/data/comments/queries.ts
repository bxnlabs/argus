import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "@/api/client";
import type { CommentsFile } from "@/types";
import { commentKeys } from "./keys";

export function useCommentsQuery(
  path: string,
  branch: string | undefined,
  baseBranch: string | null,
) {
  return useQuery({
    queryKey: commentKeys.forComparison(path, branch ?? "", baseBranch ?? ""),
    queryFn: async () => {
      const params = new URLSearchParams({
        path,
        branch: branch!,
        base: baseBranch!,
      });
      return apiFetch<CommentsFile>(`/node/api/git/comments?${params}`);
    },
    staleTime: 30_000,
    enabled: path.trim().length > 0 && !!branch && !!baseBranch,
  });
}

export function useSaveCommentsMutation(path: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (data: CommentsFile) => {
      const params = new URLSearchParams({ path });
      return apiFetch<{ status: string }>(`/node/api/git/comments?${params}`, {
        method: "POST",
        body: JSON.stringify(data),
      });
    },
    onSuccess: (_data, variables) => {
      queryClient.setQueryData(
        commentKeys.forComparison(path, variables.branch, variables.baseBranch),
        variables,
      );
    },
  });
}

export function useDeleteCommentsMutation(path: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async ({
      branch,
      baseBranch,
    }: {
      branch: string;
      baseBranch: string;
    }) => {
      const params = new URLSearchParams({
        path,
        branch,
        base: baseBranch,
      });
      return apiFetch<{ status: string }>(
        `/node/api/git/comments?${params}`,
        { method: "DELETE" },
      );
    },
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: commentKeys.forComparison(path, variables.branch, variables.baseBranch),
      });
    },
  });
}
