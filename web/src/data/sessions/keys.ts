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
  // Scope-independent prefix matching every session mutation on every node.
  // useSessionMutationState subscribes here and narrows by the *current* scope
  // on each render, which is what frees it from having to be remounted when the
  // active node changes — see the note on that hook.
  anyMutation: () => ["sessions"] as const,
  // Mutation keys carry the same scope as the query keys so in-flight state
  // never leaks across nodes. Called without a kind this yields the prefix
  // that every session mutation matches — what useSessionMutationState
  // subscribes to (TanStack matches mutation filters by prefix).
  mutation: (scope: string, kind?: SessionMutationKind) =>
    kind
      ? ([...sessionKeys.all(scope), "mutation", kind] as const)
      : ([...sessionKeys.all(scope), "mutation"] as const),
};

// Inverses of sessionKeys.mutation: recover the kind and the node scope from a
// mutation's key. Live next to the factory so the shapes cannot drift apart.
function isSessionMutationKey(
  key: readonly unknown[] | undefined,
): key is readonly unknown[] {
  return (
    !!key && key.length === 4 && key[0] === "sessions" && key[2] === "mutation"
  );
}

export function sessionMutationKind(
  key: readonly unknown[] | undefined,
): SessionMutationKind | undefined {
  if (!isSessionMutationKey(key)) return undefined;
  const kind = key[3];
  return SESSION_MUTATION_KINDS.includes(kind as SessionMutationKind)
    ? (kind as SessionMutationKind)
    : undefined;
}

export function sessionMutationScope(
  key: readonly unknown[] | undefined,
): string | undefined {
  if (!isSessionMutationKey(key)) return undefined;
  const scope = key[1];
  return typeof scope === "string" ? scope : undefined;
}

export const statusKeys = {
  all: (scope: string) => ["session-statuses", scope] as const,
};

export const profileKeys = {
  all: (scope: string) => ["profiles", scope] as const,
  list: (scope: string) => [...profileKeys.all(scope), "list"] as const,
};
