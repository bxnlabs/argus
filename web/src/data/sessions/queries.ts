import { useEffect, useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import type { QueryClient } from "@tanstack/react-query";
import {
  apiFetch,
  POLL_TIMEOUT_MS,
  SESSION_OPERATION_TIMEOUT_MS,
} from "@/api/client";
import { useActiveNode } from "@/hooks/useActiveNode";
import type { Session, ProviderType } from "@/types";
import { sessionKeys, profileKeys, statusKeys } from "./keys";

/**
 * Refresh both halves of the sidebar after the set of sessions changes.
 *
 * The list and the statuses are separate polls on separate intervals, so
 * invalidating only the list puts a session on screen that no status entry
 * covers yet — the new row draws the unlabeled muted dot `getStatusMeta`
 * falls back to, and holds it until the next status tick. Membership changes
 * are exactly when the two can disagree, so they refresh together. Renames and
 * profile edits don't need this: they change a session, not which sessions
 * exist.
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
  /** A roster fetch has answered from the server since this hook mounted. */
  everFetched: boolean;
  /** The cached roster currently reflects a completed server answer. */
  settled: boolean;
  /**
   * Increments once per server answer.
   *
   * `settled` describes a state and can be left, which makes it useless to
   * anything that needs to know an answer *happened*. A success immediately
   * followed by a mutation's optimistic write renders once, as `settled: false`
   * — the answer arrived and nothing downstream can see that it did. Consumers
   * watching for the event watch this instead.
   */
  fetchedRevision: number;
}

/**
 * Tracks what the *server* has actually said about the roster, as opposed to
 * what the query's rendered status implies.
 *
 * Anything that decides a session no longer exists needs the former, and
 * `status`/`isSuccess`/`isFetching` cannot supply it. TanStack scores an
 * optimistic `setQueryData` as a success, and rename and pin both cancel the
 * in-flight roster fetch before writing — so at the render layer that pair is
 * indistinguishable from a fetch that answered (measured: the observer emits
 * `fetching` then `idle`+`success`, and React batches the cancelled state in
 * between out of existence). Retry pausing and a fetch too fast for React to
 * commit its in-flight snapshot produce the same ambiguity from the other side.
 *
 * The cache events carry the distinction the snapshots lose: a manual write
 * arrives as `{ type: "success", manual: true }`, a real answer as the same
 * action without it. Subscribing is therefore not defensiveness, it's the only
 * place the question can be answered.
 *
 * `everFetched` latches per mount, for one-shot reconciliation. `settled` is
 * live, for repeated absence checks, and goes false whenever a fetch starts,
 * fails, or the cache is written locally — each of which makes "not in the
 * list" mean "not yet" rather than "not anymore".
 */
export function useRosterFetchState(): RosterFetchState {
  const queryClient = useQueryClient();
  const { scope } = useActiveNode();
  // The revision carries the history `settled` can't: a render that collapses a
  // success and a following write into one still shows the count going up.
  const [state, setState] = useState({ settled: false, revision: 0 });

  useEffect(() => {
    setState({ settled: false, revision: 0 });
    const key = JSON.stringify(sessionKeys.list(scope));
    return queryClient.getQueryCache().subscribe((event) => {
      if (event.type !== "updated") return;
      if (JSON.stringify(event.query.queryKey) !== key) return;
      const action = event.action;
      if (action.type === "success") {
        if (action.manual) setState((s) => ({ ...s, settled: false }));
        else setState((s) => ({ settled: true, revision: s.revision + 1 }));
      } else if (action.type === "error" || action.type === "fetch") {
        setState((s) => ({ ...s, settled: false }));
      }
    });
  }, [queryClient, scope]);

  // Derived rather than stored, so it cannot drift from the count that feeds it.
  return {
    everFetched: state.revision > 0,
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
      // Can sit on a `docker compose up` the node allows 20 minutes for, and
      // on an unbounded clone for a remote source — the default read deadline
      // would report a failure while the node was still working.
      apiFetch<CreateSessionResponse>(baseUrl, "/api/node/sessions", {
        method: "POST",
        body: JSON.stringify(input),
        timeoutMs: SESSION_OPERATION_TIMEOUT_MS,
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
        { method: "POST", timeoutMs: SESSION_OPERATION_TIMEOUT_MS },
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
        // Changing profile runs the same docker preconditions as create
        // (prepareDockerTarget), so it gets the same ceiling.
        ? apiFetch(baseUrl, `/api/node/sessions/${encodeURIComponent(sessionId)}/profile`, {
            method: "DELETE",
            timeoutMs: SESSION_OPERATION_TIMEOUT_MS,
          })
        : apiFetch(baseUrl, `/api/node/sessions/${encodeURIComponent(sessionId)}/profile`, {
            method: "PUT",
            body: JSON.stringify({ profile }),
            timeoutMs: SESSION_OPERATION_TIMEOUT_MS,
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
