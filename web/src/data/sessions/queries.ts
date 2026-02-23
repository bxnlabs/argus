import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "@/api/client";
import type { Session, AgentType } from "@/types";
import { sessionKeys } from "./keys";

interface SessionsResponse {
  sessions: Session[];
}

export function useSessionsQuery() {
  return useQuery({
    queryKey: sessionKeys.list(),
    queryFn: () => apiFetch<SessionsResponse>("/agent/api/sessions"),
    staleTime: 5000,
    refetchInterval: 10000,
  });
}

export interface CreateSessionInput {
  name?: string;
  working_directory: string;
  agent_type: AgentType;
  auto_approve: boolean;
  model?: string;
}

interface CreateSessionResponse {
  session: Session;
}

export function useCreateSession() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (input: CreateSessionInput) =>
      apiFetch<CreateSessionResponse>("/agent/api/sessions", {
        method: "POST",
        body: JSON.stringify(input),
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: sessionKeys.list() });
    },
  });
}

export function useDeleteSession() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (sessionId: string) =>
      apiFetch(`/agent/api/sessions/${sessionId}`, { method: "DELETE" }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: sessionKeys.list() });
    },
  });
}

export function useRenameSession() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({
      sessionId,
      newName,
    }: {
      sessionId: string;
      newName: string;
    }) =>
      apiFetch(`/agent/api/sessions/${sessionId}`, {
        method: "PATCH",
        body: JSON.stringify({ name: newName }),
      }),
    onMutate: async ({ sessionId, newName }) => {
      await queryClient.cancelQueries({ queryKey: sessionKeys.list() });
      const previous = queryClient.getQueryData<SessionsResponse>(
        sessionKeys.list(),
      );
      queryClient.setQueryData<SessionsResponse>(sessionKeys.list(), (old) =>
        old
          ? {
              ...old,
              sessions: old.sessions.map((s) =>
                s.id === sessionId ? { ...s, name: newName } : s,
              ),
            }
          : old,
      );
      return { previous };
    },
    onError: (_err, _vars, context) => {
      if (context?.previous) {
        queryClient.setQueryData(sessionKeys.list(), context.previous);
      }
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: sessionKeys.list() });
    },
  });
}

