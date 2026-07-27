import { useMutationState } from "@tanstack/react-query";
import { useActiveNode } from "@/hooks/useActiveNode";
import {
  sessionKeys,
  sessionMutationKind,
  type SessionMutationKind,
} from "@/data/sessions/keys";

// What a busy sidebar row is doing. Distinct from SessionMutationKind: the
// kind names the mutation, this names the row state.
export type BusyKind = "cloning" | "deleting" | "profile";

// One pending mutation, flattened to the two fields the UI cares about.
export interface PendingSessionMutation {
  kind: SessionMutationKind | undefined;
  sessionId: string | undefined;
}

export interface SessionMutationState {
  isCreating: boolean;
  busySessions: Record<string, BusyKind>;
}

const BUSY_BY_KIND: Partial<Record<SessionMutationKind, BusyKind>> = {
  clone: "cloning",
  delete: "deleting",
  profile: "profile",
};

/**
 * Shapes the pending mutation set into the two things the UI reads. Pure and
 * exported so the mapping is unit-testable without a QueryClient.
 *
 * Two mutations on the same session (e.g. a profile change then a delete) is
 * not a case the UI prevents; last one wins, which is the more urgent state.
 */
export function toBusyState(
  entries: PendingSessionMutation[],
): SessionMutationState {
  const busySessions: Record<string, BusyKind> = {};
  let isCreating = false;

  for (const { kind, sessionId } of entries) {
    if (kind === "create") {
      isCreating = true;
      continue;
    }
    const busy = kind ? BUSY_BY_KIND[kind] : undefined;
    if (busy && sessionId) busySessions[sessionId] = busy;
  }

  return { isCreating, busySessions };
}

/**
 * Which session mutations are in flight right now, derived from TanStack's
 * mutation cache rather than tracked by hand — so busy state clears itself on
 * settle (success or failure) and concurrent operations are correct for free.
 *
 * Scoped to the active node: the filter key carries the node scope, so a
 * delete on one node never marks a row busy on another.
 */
export function useSessionMutationState(): SessionMutationState {
  const { scope } = useActiveNode();

  const entries = useMutationState({
    filters: { mutationKey: sessionKeys.mutation(scope), status: "pending" },
    select: (mutation): PendingSessionMutation => ({
      kind: sessionMutationKind(mutation.options.mutationKey),
      sessionId: (mutation.state.variables as { sessionId?: string } | undefined)
        ?.sessionId,
    }),
  });

  return toBusyState(entries);
}
