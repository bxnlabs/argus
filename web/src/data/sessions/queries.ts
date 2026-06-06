import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "@/api/client";
import type { Session, ProviderType } from "@/types";
import { sessionKeys, profileKeys } from "./keys";

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

export function useCloneSession() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (sessionId: string) =>
      apiFetch<CreateSessionResponse>(
        `/node/api/sessions/${encodeURIComponent(sessionId)}/clone`,
        { method: "POST" },
      ),
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
        `/node/api/sessions/${encodeURIComponent(sessionId)}?force=true${deleteBranch ? "&delete_branch=true" : ""}`,
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
      apiFetch(`/node/api/sessions/${encodeURIComponent(sessionId)}`, {
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

export function useChangeSessionProfile() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({
      sessionId,
      profile,
    }: {
      sessionId: string;
      profile: string | null;
    }) =>
      profile === null
        ? apiFetch(`/node/api/sessions/${encodeURIComponent(sessionId)}/profile`, {
            method: "DELETE",
          })
        : apiFetch(`/node/api/sessions/${encodeURIComponent(sessionId)}/profile`, {
            method: "PUT",
            body: JSON.stringify({ profile }),
          }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: sessionKeys.list() });
    },
  });
}

export interface UpdateSessionInput {
  sessionId: string;
  pinned?: boolean;
}

export function useUpdateSession() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ sessionId, pinned }: UpdateSessionInput) =>
      apiFetch(`/node/api/sessions/${encodeURIComponent(sessionId)}`, {
        method: "PATCH",
        body: JSON.stringify({ pinned }),
      }),
    onMutate: async ({ sessionId, pinned }) => {
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
                  ? { ...s, ...(pinned !== undefined ? { pinned } : {}) }
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

