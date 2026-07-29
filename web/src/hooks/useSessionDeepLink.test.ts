import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { renderHook, act, cleanup } from "@testing-library/react";
import {
  useSessionDeepLink,
  notifySessionDeepLink,
} from "./useSessionDeepLink";
import type { Session } from "@/types";

function session(id: string): Session {
  return { id, name: id, working_directory: "/tmp" } as Session;
}

function setParam(value: string | null) {
  const url = new URL("http://localhost/");
  if (value !== null) url.searchParams.set("session", value);
  window.history.replaceState({}, "", url.pathname + url.search);
}

function currentParam() {
  return new URLSearchParams(window.location.search).get("session");
}

beforeEach(() => setParam(null));
// Not automatic in this project (no `globals: true`), and load-bearing here:
// the hook listens on `window`, so a hook left mounted by an earlier test would
// wake on this one's notify and consume the param first.
afterEach(cleanup);

describe("useSessionDeepLink", () => {
  it("attaches the linked session and clears the param", () => {
    setParam("s1");
    const onAttach = vi.fn();

    renderHook(() =>
      useSessionDeepLink({
        sessionsLoaded: true,
        sessions: [session("s1")],
        onAttach,
      }),
    );

    expect(onAttach).toHaveBeenCalledWith(expect.objectContaining({ id: "s1" }));
    expect(currentParam()).toBeNull();
  });

  it("does nothing before the session list has loaded", () => {
    setParam("s1");
    const onAttach = vi.fn();

    renderHook(() =>
      useSessionDeepLink({
        sessionsLoaded: false,
        sessions: [session("s1")],
        onAttach,
      }),
    );

    expect(onAttach).not.toHaveBeenCalled();
    expect(currentParam()).toBe("s1");
  });

  // The regression this exists for. `sessionsLoaded` is TanStack's `isSuccess`,
  // true the moment a *cached* list is available with a refetch still in
  // flight — which is exactly the state a workspace remounts into when the
  // "Open" action on a create/clone toast switches back to the node that owns
  // the session. Consuming the param on that first miss dropped the request
  // for good, and the toast was the only affordance the user had.
  it("keeps the param when the session is not in the list yet, and attaches once it appears", () => {
    setParam("s-new");
    const onAttach = vi.fn();

    const { rerender } = renderHook(
      ({ sessions }: { sessions: Session[] }) =>
        useSessionDeepLink({ sessionsLoaded: true, sessions, onAttach }),
      { initialProps: { sessions: [session("stale-1")] } },
    );

    expect(onAttach).not.toHaveBeenCalled();
    expect(currentParam()).toBe("s-new");

    // The refetch lands.
    rerender({ sessions: [session("stale-1"), session("s-new")] });

    expect(onAttach).toHaveBeenCalledWith(
      expect.objectContaining({ id: "s-new" }),
    );
    expect(currentParam()).toBeNull();
  });

  it("does not attach again on later renders once the param is cleared", () => {
    setParam("s1");
    const onAttach = vi.fn();

    const { rerender } = renderHook(
      ({ sessions }: { sessions: Session[] }) =>
        useSessionDeepLink({ sessionsLoaded: true, sessions, onAttach }),
      { initialProps: { sessions: [session("s1")] } },
    );

    expect(onAttach).toHaveBeenCalledTimes(1);
    rerender({ sessions: [session("s1"), session("s2")] });
    expect(onAttach).toHaveBeenCalledTimes(1);
  });

  // The param is the one-shot token, not an "already handled" ref: a second
  // hand-off for the same session later in the same mount has to land too.
  it("honours a second deep link to the same session in one mount", () => {
    setParam("s1");
    const onAttach = vi.fn();

    const { rerender } = renderHook(
      ({ sessions }: { sessions: Session[] }) =>
        useSessionDeepLink({ sessionsLoaded: true, sessions, onAttach }),
      { initialProps: { sessions: [session("s1")] } },
    );
    expect(onAttach).toHaveBeenCalledTimes(1);

    setParam("s1");
    rerender({ sessions: [session("s1"), session("s2")] });

    expect(onAttach).toHaveBeenCalledTimes(2);
    expect(currentParam()).toBeNull();
  });

  // The gap this closes: the "Open" action fires from an unmounted workspace's
  // closure, but the user has already navigated back to this node themselves.
  // `setActiveNode` no-ops, `replaceState` re-renders nobody, and the attach
  // would otherwise sit until some unrelated dependency happened to churn.
  it("attaches on an explicit notify with no other render trigger", () => {
    const onAttach = vi.fn();
    // Hoisted so it keeps its identity across the re-render the notify causes.
    // Built inline it would be a new array each render, and the effect would
    // re-run off that churn — passing without the notify wiring doing anything.
    const sessions = [session("s1")];

    renderHook(() =>
      useSessionDeepLink({ sessionsLoaded: true, sessions, onAttach }),
    );
    expect(onAttach).not.toHaveBeenCalled();

    // Exactly what openCreatedSession does: write the param, then announce it.
    // No prop changes, no re-render from anything else.
    act(() => {
      setParam("s1");
      notifySessionDeepLink();
    });

    expect(onAttach).toHaveBeenCalledWith(expect.objectContaining({ id: "s1" }));
    expect(currentParam()).toBeNull();
  });

  it("stops listening for notifies once unmounted", () => {
    const onAttach = vi.fn();
    const sessions = [session("s1")];
    const { unmount } = renderHook(() =>
      useSessionDeepLink({ sessionsLoaded: true, sessions, onAttach }),
    );
    unmount();

    act(() => {
      setParam("s1");
      notifySessionDeepLink();
    });

    expect(onAttach).not.toHaveBeenCalled();
    expect(currentParam()).toBe("s1");
  });

  it("ignores an absent param", () => {
    const onAttach = vi.fn();

    renderHook(() =>
      useSessionDeepLink({
        sessionsLoaded: true,
        sessions: [session("s1")],
        onAttach,
      }),
    );

    expect(onAttach).not.toHaveBeenCalled();
  });

  it("leaves other query params intact when it clears its own", () => {
    window.history.replaceState({}, "", "/?keep=1&session=s1");
    const onAttach = vi.fn();

    renderHook(() =>
      useSessionDeepLink({
        sessionsLoaded: true,
        sessions: [session("s1")],
        onAttach,
      }),
    );

    expect(currentParam()).toBeNull();
    expect(new URLSearchParams(window.location.search).get("keep")).toBe("1");
  });
});
