import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { apiFetch, apiTextFetch } from "@/api/client";
import type { Session, ProviderType, SessionStatusInfo } from "@/types";
import { sessionKeys, statusKeys, profileKeys } from "./keys";

interface StatusCache {
  statuses: Record<string, SessionStatusInfo>;
}

interface SessionsResponse {
  sessions: Session[];
  home_dir: string;
}

export function useSessionsQuery() {
  return useQuery({
    queryKey: sessionKeys.list(),
    queryFn: () => apiFetch<SessionsResponse>("/node/api/sessions"),
    staleTime: 5000,
    refetchInterval: 10000,
  });
}

export interface CreateSessionInput {
  name?: string;
  source?: string;
  provider_type: ProviderType;
  auto_approve: boolean;
  model?: string;
  profile?: string;
  branch?: string;
}

interface CreateSessionResponse {
  session: Session;
}

export function useCreateSession() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (input: CreateSessionInput) =>
      apiFetch<CreateSessionResponse>("/node/api/sessions", {
        method: "POST",
        body: JSON.stringify(input),
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: sessionKeys.list() });
    },
  });
}

interface DeleteSessionResponse {
  success: boolean;
  branch_deleted: boolean;
}

export function useDeleteSession() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({
      sessionId,
      deleteBranch,
    }: {
      sessionId: string;
      deleteBranch?: boolean;
    }) =>
      apiFetch<DeleteSessionResponse>(
        `/node/api/sessions/${sessionId}?force=true${deleteBranch ? "&delete_branch=true" : ""}`,
        { method: "DELETE" },
      ),
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
      apiFetch(`/node/api/sessions/${sessionId}`, {
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

export interface UpdateSessionInput {
  sessionId: string;
  flagged?: boolean;
  starred?: boolean;
}

export function useUpdateSession() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ sessionId, flagged, starred }: UpdateSessionInput) =>
      apiFetch(`/node/api/sessions/${sessionId}`, {
        method: "PATCH",
        body: JSON.stringify({ flagged, starred }),
      }),
    onMutate: async ({ sessionId, flagged, starred }) => {
      await queryClient.cancelQueries({ queryKey: sessionKeys.list() });
      const previous = queryClient.getQueryData<SessionsResponse>(
        sessionKeys.list(),
      );
      queryClient.setQueryData<SessionsResponse>(sessionKeys.list(), (old) =>
        old
          ? {
              ...old,
              sessions: old.sessions.map((s) =>
                s.id === sessionId
                  ? {
                      ...s,
                      ...(flagged !== undefined ? { flagged } : {}),
                      ...(starred !== undefined ? { starred } : {}),
                    }
                  : s,
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

export function useMarkRead() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (sessionId: string) =>
      apiTextFetch(
        `/node/api/sessions/${encodeURIComponent(sessionId)}/acknowledge`,
        { method: "POST" },
      ),
    onMutate: async (sessionId: string) => {
      await queryClient.cancelQueries({ queryKey: statusKeys.all });
      const previous = queryClient.getQueryData<StatusCache>(statusKeys.all);
      queryClient.setQueryData<StatusCache>(statusKeys.all, (old) => {
        const status = old?.statuses?.[sessionId];
        if (!old || !status) return old;
        return {
          ...old,
          statuses: {
            ...old.statuses,
            [sessionId]: { ...status, unreadSince: null },
          },
        };
      });
      return { previous };
    },
    onError: (_err, _vars, context) => {
      if (context?.previous) {
        queryClient.setQueryData(statusKeys.all, context.previous);
      }
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: statusKeys.all });
    },
  });
}

export function useMarkUnread() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (sessionId: string) =>
      apiTextFetch(
        `/node/api/sessions/${encodeURIComponent(sessionId)}/unread`,
        { method: "POST" },
      ),
    onMutate: async (sessionId: string) => {
      await queryClient.cancelQueries({ queryKey: statusKeys.all });
      const previous = queryClient.getQueryData<StatusCache>(statusKeys.all);
      queryClient.setQueryData<StatusCache>(statusKeys.all, (old) => {
        const status = old?.statuses?.[sessionId];
        if (!old || !status) return old;
        return {
          ...old,
          statuses: {
            ...old.statuses,
            [sessionId]: { ...status, unreadSince: new Date().toISOString() },
          },
        };
      });
      return { previous };
    },
    onError: (_err, _vars, context) => {
      if (context?.previous) {
        queryClient.setQueryData(statusKeys.all, context.previous);
      }
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: statusKeys.all });
    },
  });
}

interface ProfilesResponse {
  profiles: string[];
}

export function useProfilesQuery() {
  return useQuery({
    queryKey: profileKeys.list(),
    queryFn: () => apiFetch<ProfilesResponse>("/node/api/profiles"),
    staleTime: 30000,
  });
}

