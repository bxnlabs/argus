import { useMutationState } from "@tanstack/react-query";
import { useActiveNode } from "@/hooks/useActiveNode";
import {
  sessionKeys,
  sessionMutationKind,
  sessionMutationScope,
  type SessionMutationKind,
} from "@/data/sessions/keys";

// What a busy sidebar row is doing. Distinct from SessionMutationKind: the
// kind names the mutation, this names the row state.
export type BusyKind = "cloning" | "deleting" | "profile";

// One pending mutation, flattened to the fields the UI cares about. `scope` is
// the node it belongs to, carried through so the caller can narrow to the
// active one rather than the subscription doing it — see the hook below.
export interface PendingSessionMutation {
  scope: string | undefined;
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
 * Shapes the pending mutation set into the two things the UI reads, keeping
 * only the mutations belonging to `scope`. Pure and exported so the mapping is
 * unit-testable without a QueryClient.
 *
 * Two mutations on the same session (e.g. a profile change then a delete) is
 * not reachable through the UI: every one of them is launched from that row's
 * own actions menu, and a busy row takes no pointer input and refuses the
 * keys that open the menu. So there is no priority order to encode here —
 * should one ever land anyway, last wins, and since TanStack iterates its
 * mutation cache in insertion order that is the most recently started.
 */
export function toBusyState(
  entries: PendingSessionMutation[],
  scope: string,
): SessionMutationState {
  const busySessions: Record<string, BusyKind> = {};
  let isCreating = false;

  for (const { scope: entryScope, kind, sessionId } of entries) {
    // A delete on one node must never mark a row busy on another.
    if (entryScope !== scope) continue;
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
 * Scoped to the active node, but deliberately *not* by the subscription filter:
 * `useMutationState` computes its result at mount and thereafter only from the
 * mutation-cache subscription — it never recomputes merely because `filters`
 * changed. Filtering on the scope there would make node-safety depend on every
 * consumer being remounted on node switch (true today only because they all sit
 * under App's `<TabProvider key={scope}>`, and silently wrong for any consumer
 * hoisted above it). So subscribe to session mutations on *every* node and let
 * `toBusyState` narrow to the current scope on each render instead, which needs
 * no remount to stay correct.
 */
export function useSessionMutationState(): SessionMutationState {
  const { scope } = useActiveNode();

  const entries = useMutationState({
    filters: { mutationKey: sessionKeys.anyMutation(), status: "pending" },
    select: (mutation): PendingSessionMutation => ({
      scope: sessionMutationScope(mutation.options.mutationKey),
      kind: sessionMutationKind(mutation.options.mutationKey),
      sessionId: (mutation.state.variables as { sessionId?: string } | undefined)
        ?.sessionId,
    }),
  });

  return toBusyState(entries, scope);
}
