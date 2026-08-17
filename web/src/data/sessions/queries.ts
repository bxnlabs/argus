import { useEffect, useState } from "react";
import { useQuery, useMutation, useQueryClient, hashKey } from "@tanstack/react-query";
import type { QueryClient } from "@tanstack/react-query";
import { apiFetch, POLL_TIMEOUT_MS } from "@/api/client";
import { useActiveNode } from "@/hooks/useActiveNode";
import type { Session, ProviderType } from "@/types";
import { sessionKeys, profileKeys, statusKeys } from "./keys";

/**
 * Refresh the session list and the statuses together. They are separate polls,
 * so invalidating only the list can leave a new row with no status entry,
 * drawing the muted dot `getStatusMeta` falls back to until the next status
 * tick. A membership change is when the two can disagree.
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
    queryFn: () =>
      apiFetch<SessionsResponse>(baseUrl, "/api/node/sessions", {
        timeoutMs: POLL_TIMEOUT_MS,
      }),
    staleTime: 5000,
    refetchInterval: 10000,
  });
}

export interface RosterFetchState {
  /** The cached roster currently reflects a completed server answer. */
  settled: boolean;
  /**
   * Increments once per server answer. `settled` is a state that can be left
   * again: a success immediately followed by an optimistic write renders once,
   * as `settled: false`, and a count survives that collapse where a level does
   * not.
   */
  fetchedRevision: number;
}

/**
 * Tracks what the server has actually said about the roster, rather than what
 * the query's rendered status implies. `status`/`isSuccess`/`isFetching` cannot
 * tell an optimistic `setQueryData` from a real answer; the cache events can:
 * a manual write arrives as `{ type: "success", manual: true }`, a real answer
 * as the same action without it.
 *
 * `settled` means the cached roster is currently a completed server answer, so
 * it goes false when a fetch starts, fails, or the cache is written locally.
 * Absence then means "not yet" rather than "not anymore": a latched flag would
 * stay true over a roster a failed mutation had rolled back to an older
 * snapshot.
 */
export function useRosterFetchState(): RosterFetchState {
  const queryClient = useQueryClient();
  const { scope } = useActiveNode();
  const [state, setState] = useState({ settled: false, revision: 0 });

  useEffect(() => {
    setState({ settled: false, revision: 0 });
    // Identify the roster query by the hash TanStack already computed for it,
    // rather than re-deriving key equality here: every query in the cache
    // reaches this listener, and `hashKey` is the same function that produced
    // `queryHash`, so the two notions of identity cannot drift.
    const hash = hashKey(sessionKeys.list(scope));
    return queryClient.getQueryCache().subscribe((event) => {
      if (event.type !== "updated") return;
      if (event.query.queryHash !== hash) return;
      const action = event.action;
      if (action.type === "success") {
        if (action.manual) setState((s) => ({ ...s, settled: false }));
        else setState((s) => ({ settled: true, revision: s.revision + 1 }));
      } else if (action.type === "error" || action.type === "fetch") {
        setState((s) => ({ ...s, settled: false }));
      }
    });
  }, [queryClient, scope]);

  return {
    settled: state.settled,
    fetchedRevision: state.revision,
  };
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
      // No deadline: a remote source runs `git clone` through git.Run
      // (internal/git/utils.go), a bare exec with no context, so nothing on the
      // server bounds this. A ceiling set too low reports a failure for a
      // create that then succeeds, and the retry makes a second session.
      apiFetch<CreateSessionResponse>(baseUrl, "/api/node/sessions", {
        method: "POST",
        body: JSON.stringify(input),
        timeoutMs: null,
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
      // No deadline. Clone hands Create the source session's working directory
      // (internal/node/session/lifecycle.go), a local path, so it does not
      // reach the remote `git clone`, and it creates no worktree — the managed
      // one is recognised and reused. It does run git probes (FindMainRepo,
      // FindWorktreeByPath, the remote URL read) through the same context-less
      // exec, and takes the source session's lock first. Neither is bounded.
      apiFetch<CreateSessionResponse>(
        baseUrl,
        `/api/node/sessions/${encodeURIComponent(sessionId)}/clone`,
        { method: "POST", timeoutMs: null },
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
      // No deadline. The two `pre_destroy` hooks at 30s apiece are the only
      // part the node bounds; worktree removal and branch deletion run through
      // the context-less git.Run (internal/git/worktree/manager.go), and the
      // session lock ahead of them is unbounded too. Cutting the request would
      // not stop the deletion.
      apiFetch<DeleteSessionResponse>(
        baseUrl,
        `/api/node/sessions/${encodeURIComponent(sessionId)}?force=true${deleteBranch ? "&delete_branch=true" : ""}`,
        { method: "DELETE", timeoutMs: null },
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
      // No deadline. `stackOpTimeout` bounds one compose call, and a Docker
      // target can bring up two — once before teardown, once through respawn —
      // around hooks the node allows 30s apiece, after a `profileLock` wait
      // outside any context (internal/node/session/docker.go, lifecycle.go). A
      // host target runs no compose at all and the hooks are conditional, so
      // there is no aggregate bound for a ceiling here to sit above.
      profile === null
        ? apiFetch(baseUrl, `/api/node/sessions/${encodeURIComponent(sessionId)}/profile`, {
            method: "DELETE",
            timeoutMs: null,
          })
        : apiFetch(baseUrl, `/api/node/sessions/${encodeURIComponent(sessionId)}/profile`, {
            method: "PUT",
            body: JSON.stringify({ profile }),
            timeoutMs: null,
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
