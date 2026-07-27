// scope ("<id>:<url>", from useActiveNode) is the first key segment so each
// node's data is a distinct cache entry; switching nodes addresses a different
// key rather than relying on eviction.

export type SessionMutationKind = "create" | "clone" | "delete" | "profile";

const SESSION_MUTATION_KINDS: readonly SessionMutationKind[] = [
  "create",
  "clone",
  "delete",
  "profile",
];

export const sessionKeys = {
  all: (scope: string) => ["sessions", scope] as const,
  list: (scope: string) => [...sessionKeys.all(scope), "list"] as const,
  // Mutation keys carry the same scope as the query keys so in-flight state
  // never leaks across nodes. Called without a kind this yields the prefix
  // that every session mutation matches — what useSessionMutationState
  // subscribes to (TanStack matches mutation filters by prefix).
  mutation: (scope: string, kind?: SessionMutationKind) =>
    kind
      ? ([...sessionKeys.all(scope), "mutation", kind] as const)
      : ([...sessionKeys.all(scope), "mutation"] as const),
};

// Inverse of sessionKeys.mutation: recovers the kind from a mutation's key.
// Lives next to the factory so the two shapes cannot drift apart.
export function sessionMutationKind(
  key: readonly unknown[] | undefined,
): SessionMutationKind | undefined {
  if (!key || key.length !== 4) return undefined;
  if (key[0] !== "sessions" || key[2] !== "mutation") return undefined;
  const kind = key[3];
  return SESSION_MUTATION_KINDS.includes(kind as SessionMutationKind)
    ? (kind as SessionMutationKind)
    : undefined;
}

export const statusKeys = {
  all: (scope: string) => ["session-statuses", scope] as const,
};

export const profileKeys = {
  all: (scope: string) => ["profiles", scope] as const,
  list: (scope: string) => [...profileKeys.all(scope), "list"] as const,
};
