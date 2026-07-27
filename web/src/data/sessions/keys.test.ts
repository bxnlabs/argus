import { describe, it, expect } from "vitest";
import { sessionKeys, sessionMutationKind } from "./keys";

describe("sessionKeys.mutation", () => {
  it("builds the scoped prefix when no kind is given", () => {
    expect(sessionKeys.mutation("local:")).toEqual([
      "sessions",
      "local:",
      "mutation",
    ]);
  });

  it("appends the kind", () => {
    expect(sessionKeys.mutation("local:", "delete")).toEqual([
      "sessions",
      "local:",
      "mutation",
      "delete",
    ]);
  });

  // The hook subscribes with the kindless form and relies on TanStack's
  // prefix matching to catch every kinded mutation.
  it("keeps the kindless form a prefix of every kinded form", () => {
    const prefix = sessionKeys.mutation("local:");
    for (const kind of ["create", "clone", "delete", "profile"] as const) {
      const full = sessionKeys.mutation("local:", kind);
      expect(full.slice(0, prefix.length)).toEqual([...prefix]);
    }
  });

  it("scopes separately per node", () => {
    expect(sessionKeys.mutation("local:", "clone")).not.toEqual(
      sessionKeys.mutation("m1:http://gpu:80", "clone"),
    );
  });
});

describe("sessionMutationKind", () => {
  it("round-trips every kind", () => {
    for (const kind of ["create", "clone", "delete", "profile"] as const) {
      expect(sessionMutationKind(sessionKeys.mutation("local:", kind))).toBe(kind);
    }
  });

  it("returns undefined for the bare prefix", () => {
    expect(sessionMutationKind(sessionKeys.mutation("local:"))).toBeUndefined();
  });

  it("returns undefined for a session query key", () => {
    expect(sessionMutationKind(sessionKeys.list("local:"))).toBeUndefined();
  });

  it("returns undefined for another feature's mutation key", () => {
    expect(
      sessionMutationKind(["files", "local:", "mutation", "delete"]),
    ).toBeUndefined();
  });

  it("returns undefined for an unrecognised kind", () => {
    expect(
      sessionMutationKind(["sessions", "local:", "mutation", "explode"]),
    ).toBeUndefined();
  });

  it("returns undefined when the mutation has no key", () => {
    expect(sessionMutationKind(undefined)).toBeUndefined();
  });
});
