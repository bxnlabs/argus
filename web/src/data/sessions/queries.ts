import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import type { QueryClient } from "@tanstack/react-query";
import { apiFetch } from "@/api/client";
import { useActiveNode } from "@/hooks/useActiveNode";
import type { Session, ProviderType } from "@/types";
import { sessionKeys, profileKeys, statusKeys } from "./keys";

/**
 * Refresh both halves of the sidebar after the set of sessions changes.
 *
 * The list and the statuses are separate polls on separate intervals, so
 * invalidating only the list puts a session on screen that no status entry
 * covers yet — and anything reading the two together (the sidebar's summary
 * line) counts it as nothing until the next status tick lands. Membership
 * changes are exactly when the two can disagree, so they refresh together.
 * Renames and profile edits don't need this: they change a session, not which
 * sessions exist.
 */
function invalidateSessionMembership(queryClient: QueryClient, scope: string) {
  queryClient.invalidateQueries({ queryKey: sessionKeys.list(scope) });
  queryClient.invalidateQueries({ queryKey: statusKeys.all(scope) });
}

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
    mutationKey: sessionKeys.mutation(scope, "create"),
    mutationFn: (input: CreateSessionInput) =>
      apiFetch<CreateSessionResponse>(baseUrl, "/api/node/sessions", {
        method: "POST",
        body: JSON.stringify(input),
      }),
    onSuccess: () => {
      invalidateSessionMembership(queryClient, scope);
    },
  });
}

export function useCloneSession() {
  const queryClient = useQueryClient();
  const { scope, baseUrl } = useActiveNode();

  return useMutation({
    mutationKey: sessionKeys.mutation(scope, "clone"),
    mutationFn: ({ sessionId }: { sessionId: string }) =>
      apiFetch<CreateSessionResponse>(
        baseUrl,
        `/api/node/sessions/${encodeURIComponent(sessionId)}/clone`,
        { method: "POST" },
      ),
    onSuccess: () => {
      invalidateSessionMembership(queryClient, scope);
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
    mutationKey: sessionKeys.mutation(scope, "delete"),
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
    onSuccess: (_data, { sessionId }) => {
      // Drop the row immediately rather than waiting for the refetch. Without
      // this the row un-busies and flashes back to normal for a frame before
      // the invalidation removes it.
      queryClient.setQueryData<SessionsResponse>(sessionKeys.list(scope), (old) =>
        old
          ? { ...old, sessions: old.sessions.filter((s) => s.id !== sessionId) }
          : old,
      );
      invalidateSessionMembership(queryClient, scope);
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
    mutationKey: sessionKeys.mutation(scope, "profile"),
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

export type ProfileType = "host" | "docker";

export interface ProfileInfo {
  name: string;
  type: ProfileType;
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
