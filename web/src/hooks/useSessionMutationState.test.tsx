import { describe, it, expect } from "vitest";
import { useState } from "react";
import { renderHook, waitFor, act } from "@testing-library/react";
import {
  QueryClient,
  QueryClientProvider,
  useMutation,
} from "@tanstack/react-query";
import { StubNodeProvider } from "@/test/node-context";
import { sessionKeys } from "@/data/sessions/keys";
import { toBusyState, useSessionMutationState } from "./useSessionMutationState";

describe("toBusyState", () => {
  it("maps each session-scoped kind to its busy value", () => {
    expect(
      toBusyState([
        { kind: "delete", sessionId: "a" },
        { kind: "clone", sessionId: "b" },
        { kind: "profile", sessionId: "c" },
      ]),
    ).toEqual({
      isCreating: false,
      busySessions: { a: "deleting", b: "cloning", c: "profile" },
    });
  });

  it("routes create to isCreating, since it has no session id yet", () => {
    expect(toBusyState([{ kind: "create", sessionId: undefined }])).toEqual({
      isCreating: true,
      busySessions: {},
    });
  });

  it("tracks concurrent deletes independently", () => {
    const { busySessions } = toBusyState([
      { kind: "delete", sessionId: "a" },
      { kind: "delete", sessionId: "b" },
    ]);
    expect(busySessions).toEqual({ a: "deleting", b: "deleting" });
  });

  it("ignores entries missing a kind or a session id", () => {
    expect(
      toBusyState([
        { kind: undefined, sessionId: "a" },
        { kind: "delete", sessionId: undefined },
      ]),
    ).toEqual({ isCreating: false, busySessions: {} });
  });

  it("returns an empty state for no pending mutations", () => {
    expect(toBusyState([])).toEqual({ isCreating: false, busySessions: {} });
  });
});

describe("useSessionMutationState", () => {
  function wrapper({ children }: { children: React.ReactNode }) {
    // Held in state so a re-render of the wrapper can never swap the client
    // out from under the provider mid-test.
    const [qc] = useState(() => new QueryClient());
    return (
      <QueryClientProvider client={qc}>
        <StubNodeProvider>{children}</StubNodeProvider>
      </QueryClientProvider>
    );
  }

  // StubNodeProvider pins the active node to scope "local:".
  it("reports a pending session mutation from the mutation cache", async () => {
    const { result } = renderHook(
      () => {
        const state = useSessionMutationState();
        const del = useMutation({
          mutationKey: sessionKeys.mutation("local:", "delete"),
          // Never settles, so the mutation stays pending for the assertion.
          mutationFn: (_vars: { sessionId: string }) =>
            new Promise<void>(() => {}),
        });
        return { state, del };
      },
      { wrapper },
    );

    expect(result.current.state.busySessions).toEqual({});
    expect(result.current.state.isCreating).toBe(false);

    act(() => {
      result.current.del.mutate({ sessionId: "s1" });
    });

    await waitFor(() =>
      expect(result.current.state.busySessions).toEqual({ s1: "deleting" }),
    );
  });

  it("ignores mutations belonging to another node's scope", async () => {
    const { result } = renderHook(
      () => {
        const state = useSessionMutationState();
        const del = useMutation({
          mutationKey: sessionKeys.mutation("m1:http://gpu:80", "delete"),
          mutationFn: (_vars: { sessionId: string }) =>
            new Promise<void>(() => {}),
        });
        return { state, del };
      },
      { wrapper },
    );

    act(() => {
      result.current.del.mutate({ sessionId: "s1" });
    });

    await waitFor(() => expect(result.current.del.isPending).toBe(true));
    expect(result.current.state.busySessions).toEqual({});
  });
});
