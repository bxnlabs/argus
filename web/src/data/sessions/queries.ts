import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "@/api/client";
import { useActiveNode } from "@/hooks/useActiveNode";
import type { Session, ProviderType } from "@/types";
import { sessionKeys, profileKeys } from "./keys";

interface SessionsResponse {
  sessions: Session[];
  home_dir: string;
}

export function useSessionsQuery() {
  const { scope, baseUrl } = useActiveNode();
  return useQuery({
    queryKey: sessionKeys.list(scope),
    queryFn: () => apiFetch<SessionsResponse>(baseUrl, "/api/node/sessions"),
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
  const { scope, baseUrl } = useActiveNode();

  return useMutation({
    mutationFn: (input: CreateSessionInput) =>
      apiFetch<CreateSessionResponse>(baseUrl, "/api/node/sessions", {
        method: "POST",
        body: JSON.stringify(input),
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: sessionKeys.list(scope) });
    },
  });
}

export function useCloneSession() {
  const queryClient = useQueryClient();
  const { scope, baseUrl } = useActiveNode();

  return useMutation({
    mutationFn: (sessionId: string) =>
      apiFetch<CreateSessionResponse>(
        baseUrl,
        `/api/node/sessions/${encodeURIComponent(sessionId)}/clone`,
        { method: "POST" },
      ),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: sessionKeys.list(scope) });
    },
  });
}

interface DeleteSessionResponse {
  success: boolean;
  branch_deleted: boolean;
}

export function useDeleteSession() {
  const queryClient = useQueryClient();
  const { scope, baseUrl } = useActiveNode();

  return useMutation({
    mutationFn: ({
      sessionId,
      deleteBranch,
    }: {
      sessionId: string;
      deleteBranch?: boolean;
    }) =>
      apiFetch<DeleteSessionResponse>(
        baseUrl,
        `/api/node/sessions/${encodeURIComponent(sessionId)}?force=true${deleteBranch ? "&delete_branch=true" : ""}`,
        { method: "DELETE" },
      ),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: sessionKeys.list(scope) });
    },
  });
}

export function useRenameSession() {
  const queryClient = useQueryClient();
  const { scope, baseUrl } = useActiveNode();

  return useMutation({
    mutationFn: ({
      sessionId,
      newName,
    }: {
      sessionId: string;
      newName: string;
    }) =>
      apiFetch(baseUrl, `/api/node/sessions/${encodeURIComponent(sessionId)}`, {
        method: "PATCH",
        body: JSON.stringify({ name: newName }),
      }),
    onMutate: async ({ sessionId, newName }) => {
      await queryClient.cancelQueries({ queryKey: sessionKeys.list(scope) });
      const previous = queryClient.getQueryData<SessionsResponse>(
        sessionKeys.list(scope),
      );
      queryClient.setQueryData<SessionsResponse>(sessionKeys.list(scope), (old) =>
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
        queryClient.setQueryData(sessionKeys.list(scope), context.previous);
      }
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: sessionKeys.list(scope) });
    },
  });
}

export function useChangeSessionProfile() {
  const queryClient = useQueryClient();
  const { scope, baseUrl } = useActiveNode();

  return useMutation({
    mutationFn: ({
      sessionId,
      profile,
    }: {
      sessionId: string;
      profile: string | null;
    }) =>
      profile === null
        ? apiFetch(baseUrl, `/api/node/sessions/${encodeURIComponent(sessionId)}/profile`, {
            method: "DELETE",
          })
        : apiFetch(baseUrl, `/api/node/sessions/${encodeURIComponent(sessionId)}/profile`, {
            method: "PUT",
            body: JSON.stringify({ profile }),
          }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: sessionKeys.list(scope) });
    },
  });
}

export interface UpdateSessionInput {
  sessionId: string;
  pinned?: boolean;
}

export function useUpdateSession() {
  const queryClient = useQueryClient();
  const { scope, baseUrl } = useActiveNode();

  return useMutation({
    mutationFn: ({ sessionId, pinned }: UpdateSessionInput) =>
      apiFetch(baseUrl, `/api/node/sessions/${encodeURIComponent(sessionId)}`, {
        method: "PATCH",
        body: JSON.stringify({ pinned }),
      }),
    onMutate: async ({ sessionId, pinned }) => {
      await queryClient.cancelQueries({ queryKey: sessionKeys.list(scope) });
      const previous = queryClient.getQueryData<SessionsResponse>(
        sessionKeys.list(scope),
      );
      queryClient.setQueryData<SessionsResponse>(sessionKeys.list(scope), (old) =>
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
        queryClient.setQueryData(sessionKeys.list(scope), context.previous);
      }
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: sessionKeys.list(scope) });
    },
  });
}

export interface ProfileInfo {
  name: string;
  dockerized: boolean;
}

interface ProfilesResponse {
  profiles: ProfileInfo[];
}

export function useProfilesQuery() {
  const { scope, baseUrl } = useActiveNode();
  return useQuery({
    queryKey: profileKeys.list(scope),
    queryFn: () => apiFetch<ProfilesResponse>(baseUrl, "/api/node/profiles"),
    staleTime: 30000,
  });
}
