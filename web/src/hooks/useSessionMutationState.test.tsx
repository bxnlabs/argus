import { describe, it, expect } from "vitest";
import { useState } from "react";
import { renderHook, waitFor, act } from "@testing-library/react";
import {
  QueryClient,
  QueryClientProvider,
  useMutation,
} from "@tanstack/react-query";
import { StubNodeProvider } from "@/test/node-context";
import { NodeContext } from "@/contexts/NodeContext";
import { sessionKeys } from "@/data/sessions/keys";
import type { NodeWithStatus } from "@/types";
import { toBusyState, useSessionMutationState } from "./useSessionMutationState";

const SCOPE = "local:";

describe("toBusyState", () => {
  it("maps each session-scoped kind to its busy value", () => {
    expect(
      toBusyState(
        [
          { scope: SCOPE, kind: "delete", sessionId: "a" },
          { scope: SCOPE, kind: "clone", sessionId: "b" },
          { scope: SCOPE, kind: "profile", sessionId: "c" },
        ],
        SCOPE,
      ),
    ).toEqual({
      isCreating: false,
      busySessions: { a: "deleting", b: "cloning", c: "profile" },
    });
  });

  it("routes create to isCreating, since it has no session id yet", () => {
    expect(
      toBusyState([{ scope: SCOPE, kind: "create", sessionId: undefined }], SCOPE),
    ).toEqual({
      isCreating: true,
      busySessions: {},
    });
  });

  it("tracks concurrent deletes independently", () => {
    const { busySessions } = toBusyState(
      [
        { scope: SCOPE, kind: "delete", sessionId: "a" },
        { scope: SCOPE, kind: "delete", sessionId: "b" },
      ],
      SCOPE,
    );
    expect(busySessions).toEqual({ a: "deleting", b: "deleting" });
  });

  it("ignores entries missing a kind or a session id", () => {
    expect(
      toBusyState(
        [
          { scope: SCOPE, kind: undefined, sessionId: "a" },
          { scope: SCOPE, kind: "delete", sessionId: undefined },
        ],
        SCOPE,
      ),
    ).toEqual({ isCreating: false, busySessions: {} });
  });

  // The subscription deliberately spans every node, so this filter — not the
  // mutation-cache filter — is what keeps one node's work off another's rows.
  it("drops entries belonging to another node's scope", () => {
    expect(
      toBusyState(
        [
          { scope: "m1:http://gpu:80", kind: "delete", sessionId: "a" },
          { scope: "m1:http://gpu:80", kind: "create", sessionId: undefined },
          { scope: SCOPE, kind: "clone", sessionId: "b" },
        ],
        SCOPE,
      ),
    ).toEqual({ isCreating: false, busySessions: { b: "cloning" } });
  });

  it("drops entries whose scope could not be recovered from the key", () => {
    expect(
      toBusyState([{ scope: undefined, kind: "delete", sessionId: "a" }], SCOPE),
    ).toEqual({ isCreating: false, busySessions: {} });
  });

  it("returns an empty state for no pending mutations", () => {
    expect(toBusyState([], SCOPE)).toEqual({
      isCreating: false,
      busySessions: {},
    });
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

  // The regression this guards: `useMutationState` snapshots at mount and then
  // only recomputes from its cache subscription, so a scope carried in the
  // *filter* would never be re-read. That made node-safety contingent on every
  // consumer being remounted on node switch — an invariant nothing enforced.
  // Here the hook stays mounted across the switch, which is exactly the case a
  // filter-scoped subscription gets wrong.
  it("follows the active node without being remounted", async () => {
    const gpuNode: NodeWithStatus = {
      id: "m1",
      name: "gpu",
      url: "http://gpu:80",
      source: "manual",
      self: false,
      summary: null,
      online: true,
      pending: false,
    };
    const localNode: NodeWithStatus = { ...gpuNode, id: "local", name: "this", url: "", self: true };

    let switchNode!: (node: NodeWithStatus) => void;

    function SwitchableNodeProvider({ children }: { children: React.ReactNode }) {
      const [qc] = useState(() => new QueryClient());
      const [activeNode, setNode] = useState(localNode);
      switchNode = setNode;
      return (
        <QueryClientProvider client={qc}>
          <NodeContext.Provider
            value={{
              nodes: [localNode, gpuNode],
              isLoaded: true,
              activeNodeId: activeNode.id,
              activeNode,
              setActiveNode: () => {},
            }}
          >
            {children}
          </NodeContext.Provider>
        </QueryClientProvider>
      );
    }

    const { result } = renderHook(
      () => {
        const state = useSessionMutationState();
        const create = useMutation({
          mutationKey: sessionKeys.mutation("local:", "create"),
          mutationFn: () => new Promise<void>(() => {}),
        });
        const gpuDelete = useMutation({
          mutationKey: sessionKeys.mutation("m1:http://gpu:80", "delete"),
          mutationFn: (_vars: { sessionId: string }) =>
            new Promise<void>(() => {}),
        });
        return { state, create, gpuDelete };
      },
      { wrapper: SwitchableNodeProvider },
    );

    // Both dispatched while local is active, so every cache event this test
    // produces lands before the switch. Nothing after it wakes the
    // subscription — which is the point.
    act(() => {
      result.current.create.mutate();
      result.current.gpuDelete.mutate({ sessionId: "s1" });
    });
    await waitFor(() => expect(result.current.state.isCreating).toBe(true));
    expect(result.current.state.busySessions).toEqual({});

    // Same mounted hook, different active node, no intervening cache event.
    // The local create is still pending but is no longer this node's business,
    // and gpu's delete has been pending the whole time and is now visible.
    act(() => switchNode(gpuNode));
    expect(result.current.state.isCreating).toBe(false);
    expect(result.current.state.busySessions).toEqual({ s1: "deleting" });

    act(() => switchNode(localNode));
    expect(result.current.state.isCreating).toBe(true);
    expect(result.current.state.busySessions).toEqual({});
  });
});
