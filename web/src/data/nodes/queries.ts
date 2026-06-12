import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { addNode, deleteNode, fetchNodes, updateNode } from "./api";
import { nodeKeys } from "./keys";

export function useNodesQuery() {
  return useQuery({
    queryKey: nodeKeys.list(),
    queryFn: fetchNodes,
    staleTime: 10_000,
    refetchInterval: 15_000,
  });
}

export function useAddNode() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ name, url }: { name: string; url: string }) => addNode(name, url),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: nodeKeys.list() }),
  });
}

export function useUpdateNode() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, name, url }: { id: string; name: string; url: string }) =>
      updateNode(id, name, url),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: nodeKeys.list() }),
  });
}

export function useDeleteNode() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => deleteNode(id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: nodeKeys.list() }),
  });
}
