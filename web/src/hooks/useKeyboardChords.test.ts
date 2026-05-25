import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, act, cleanup } from "@testing-library/react";

// The platform helper is mocked per-suite so we can exercise both the macOS
// (metaKey) and non-macOS (ctrlKey) leader branches deterministically.
const isMacMock = vi.fn<() => boolean>(() => true);
vi.mock("@/lib/device", () => ({ isMac: () => isMacMock() }));

import { useKeyboardChords, type ChordMap } from "./useKeyboardChords";

// --- Test helpers ---

/** Dispatch a real keydown on `document` in the capture phase and return it so
 *  callers can assert `defaultPrevented`. Wrapped in `act` by the caller. */
function dispatchKey(
  key: string,
  init: Partial<KeyboardEventInit> = {},
): KeyboardEvent {
  const event = new KeyboardEvent("keydown", {
    key,
    bubbles: true,
    cancelable: true,
    ...init,
  });
  document.dispatchEvent(event);
  return event;
}

/** Fire the leader combo for the currently-mocked platform. */
function dispatchLeader(): KeyboardEvent {
  return dispatchKey(";", isMacMock() ? { metaKey: true } : { ctrlKey: true });
}

function makeBindings() {
  const n = vi.fn();
  const gRun = vi.fn();
  const h = vi.fn();
  const c = vi.fn();
  const bindings: ChordMap = {
    n: { label: "New", run: n },
    g: {
      label: "Go",
      run: gRun,
      children: {
        h: { label: "Home", run: h },
        c: { label: "Compare", run: c },
      },
    },
  };
  return { bindings, n, gRun, h, c };
}

beforeEach(() => {
  isMacMock.mockReturnValue(true);
  vi.useRealTimers();
});

afterEach(() => {
  // Globals aren't enabled in this project, so Testing Library's automatic
  // afterEach(cleanup) never registers — unmount mounted hooks by hand so their
  // document/window listeners don't bleed into the next test.
  cleanup();
  vi.useRealTimers();
  vi.clearAllMocks();
});

describe("useKeyboardChords", () => {
  it("dispatches a prefix action and returns to idle", () => {
    const { bindings, n } = makeBindings();
    const { result } = renderHook(() => useKeyboardChords(bindings));

    act(() => {
      dispatchLeader();
    });
    expect(result.current.pending).not.toBeNull();

    act(() => {
      dispatchKey("n");
    });

    expect(n).toHaveBeenCalledTimes(1);
    expect(result.current.pending).toBeNull();
  });

  it("handles the g group: run + sub-chord, then a child action", () => {
    const { bindings, gRun, h, c } = makeBindings();
    const { result } = renderHook(() => useKeyboardChords(bindings));

    // leader -> g fires g.run AND enters the sub level
    act(() => {
      dispatchLeader();
    });
    act(() => {
      dispatchKey("g");
    });
    expect(gRun).toHaveBeenCalledTimes(1);
    expect(result.current.pending?.path).toEqual(["g"]);
    expect(result.current.pending?.level).toBe(bindings.g.children);

    // g -> h fires the child action and returns to idle
    act(() => {
      dispatchKey("h");
    });
    expect(h).toHaveBeenCalledTimes(1);
    expect(result.current.pending).toBeNull();

    // Fresh run: leader -> g -> c fires the other child
    act(() => {
      dispatchLeader();
    });
    act(() => {
      dispatchKey("g");
    });
    act(() => {
      dispatchKey("c");
    });
    expect(c).toHaveBeenCalledTimes(1);
    expect(result.current.pending).toBeNull();
  });

  it("auto-cancels after the timeout elapses", () => {
    vi.useFakeTimers();
    const { bindings, n } = makeBindings();
    const { result } = renderHook(() => useKeyboardChords(bindings));

    act(() => {
      dispatchLeader();
    });
    expect(result.current.pending).not.toBeNull();

    act(() => {
      vi.advanceTimersByTime(1300);
    });
    expect(result.current.pending).toBeNull();

    // A follow-up key after expiry must not act.
    act(() => {
      dispatchKey("n");
    });
    expect(n).not.toHaveBeenCalled();
  });

  it("cancels on Escape without firing anything", () => {
    const { bindings, n, gRun } = makeBindings();
    const { result } = renderHook(() => useKeyboardChords(bindings));

    act(() => {
      dispatchLeader();
    });
    act(() => {
      dispatchKey("Escape");
    });

    expect(result.current.pending).toBeNull();
    expect(n).not.toHaveBeenCalled();
    expect(gRun).not.toHaveBeenCalled();
  });

  it("cancels silently on an unmapped follow-up key", () => {
    const { bindings, n, gRun } = makeBindings();
    const { result } = renderHook(() => useKeyboardChords(bindings));

    act(() => {
      dispatchLeader();
    });
    act(() => {
      dispatchKey("z");
    });

    expect(result.current.pending).toBeNull();
    expect(n).not.toHaveBeenCalled();
    expect(gRun).not.toHaveBeenCalled();
  });

  it("arms on Cmd+; but not Ctrl+; on macOS", () => {
    isMacMock.mockReturnValue(true);
    const { bindings } = makeBindings();
    const { result } = renderHook(() => useKeyboardChords(bindings));

    act(() => {
      dispatchKey(";", { ctrlKey: true });
    });
    expect(result.current.pending).toBeNull();

    act(() => {
      dispatchKey(";", { metaKey: true });
    });
    expect(result.current.pending).not.toBeNull();
  });

  it("arms on Ctrl+; but not Cmd+; off macOS", () => {
    isMacMock.mockReturnValue(false);
    const { bindings } = makeBindings();
    const { result } = renderHook(() => useKeyboardChords(bindings));

    act(() => {
      dispatchKey(";", { metaKey: true });
    });
    expect(result.current.pending).toBeNull();

    act(() => {
      dispatchKey(";", { ctrlKey: true });
    });
    expect(result.current.pending).not.toBeNull();
  });

  it("captures follow-up keys airtight (mapped and unmapped)", () => {
    const { bindings } = makeBindings();
    renderHook(() => useKeyboardChords(bindings));

    // Mapped follow-up
    act(() => {
      dispatchLeader();
    });
    let event!: KeyboardEvent;
    act(() => {
      event = dispatchKey("n");
    });
    expect(event.defaultPrevented).toBe(true);

    // Unmapped follow-up
    act(() => {
      dispatchLeader();
    });
    act(() => {
      event = dispatchKey("z");
    });
    expect(event.defaultPrevented).toBe(true);
  });

  it("does not prevent keys while idle", () => {
    const { bindings } = makeBindings();
    renderHook(() => useKeyboardChords(bindings));

    let event!: KeyboardEvent;
    act(() => {
      event = dispatchKey("n");
    });
    expect(event.defaultPrevented).toBe(false);
  });

  it("exposes the pending state at the prefix and sub levels", () => {
    const { bindings } = makeBindings();
    const { result } = renderHook(() => useKeyboardChords(bindings));

    act(() => {
      dispatchLeader();
    });
    expect(result.current.pending).toEqual({ path: [], level: bindings });
    expect(result.current.pending?.level).toBe(bindings);

    act(() => {
      dispatchKey("g");
    });
    expect(result.current.pending?.path).toEqual(["g"]);
    expect(result.current.pending?.level).toBe(bindings.g.children);
  });

  it("is inert when disabled", () => {
    const { bindings } = makeBindings();
    const { result } = renderHook(() =>
      useKeyboardChords(bindings, { enabled: false }),
    );

    let event!: KeyboardEvent;
    act(() => {
      event = dispatchLeader();
    });
    expect(result.current.pending).toBeNull();
    expect(event.defaultPrevented).toBe(false);
  });

  it("cancels pending state on window blur", () => {
    const { bindings } = makeBindings();
    const { result } = renderHook(() => useKeyboardChords(bindings));

    act(() => {
      dispatchLeader();
    });
    expect(result.current.pending).not.toBeNull();

    act(() => {
      window.dispatchEvent(new Event("blur"));
    });
    expect(result.current.pending).toBeNull();
  });

  it("ignores a bare modifier keydown while pending", () => {
    const { bindings, n } = makeBindings();
    const { result } = renderHook(() => useKeyboardChords(bindings));

    act(() => {
      dispatchLeader();
    });
    // The user is still holding Cmd from the leader; a lone Shift must not cancel.
    act(() => {
      dispatchKey("Shift", { shiftKey: true });
    });
    expect(result.current.pending).not.toBeNull();

    // The real follow-up still fires.
    act(() => {
      dispatchKey("n");
    });
    expect(n).toHaveBeenCalledTimes(1);
    expect(result.current.pending).toBeNull();
  });

  it("reads the latest bindings through a ref (no stale run)", () => {
    const first = vi.fn();
    const second = vi.fn();
    const make = (run: () => void): ChordMap => ({ n: { label: "New", run } });

    const { result, rerender } = renderHook(
      ({ bindings }: { bindings: ChordMap }) => useKeyboardChords(bindings),
      { initialProps: { bindings: make(first) } },
    );

    rerender({ bindings: make(second) });

    act(() => {
      dispatchLeader();
    });
    act(() => {
      dispatchKey("n");
    });

    expect(first).not.toHaveBeenCalled();
    expect(second).toHaveBeenCalledTimes(1);
    expect(result.current.pending).toBeNull();
  });

  it("matches uppercase letters by lowercasing single chars", () => {
    const { bindings, n } = makeBindings();
    renderHook(() => useKeyboardChords(bindings));

    act(() => {
      dispatchLeader();
    });
    // Shift held -> e.key is "N"; should still match the lowercase binding.
    act(() => {
      dispatchKey("N", { shiftKey: true });
    });
    expect(n).toHaveBeenCalledTimes(1);
  });

  it("does not cancel pending state on a repeated leader keydown", () => {
    // Holding the leader emits auto-repeated keydowns; the second one must be
    // ignored so the chord stays armed.
    const { bindings } = makeBindings();
    const { result } = renderHook(() => useKeyboardChords(bindings));

    act(() => {
      dispatchLeader();
    });
    expect(result.current.pending).not.toBeNull();

    // Simulate the OS auto-repeating the leader combo.
    act(() => {
      dispatchKey(";", isMacMock() ? { metaKey: true, repeat: true } : { ctrlKey: true, repeat: true });
    });
    // Chord must still be armed.
    expect(result.current.pending).not.toBeNull();
  });

  it("does not prevent repeated keydowns while idle", () => {
    // Mirrors the existing "does not prevent keys while idle" test but with
    // repeat:true — repeat events should pass through completely when idle.
    const { bindings } = makeBindings();
    renderHook(() => useKeyboardChords(bindings));

    let event!: KeyboardEvent;
    act(() => {
      event = dispatchKey("n", { repeat: true });
    });
    expect(event.defaultPrevented).toBe(false);
  });

  it("does not cancel pending state on an IME composition keydown", () => {
    // While pending, a key emitted during IME composition must not cancel the
    // chord or consume the event.
    const { bindings } = makeBindings();
    const { result } = renderHook(() => useKeyboardChords(bindings));

    act(() => {
      dispatchLeader();
    });
    expect(result.current.pending).not.toBeNull();

    let event!: KeyboardEvent;
    act(() => {
      event = dispatchKey("a", { isComposing: true });
    });
    expect(result.current.pending).not.toBeNull();
    expect(event.defaultPrevented).toBe(false);
  });

  it("suppresses sibling capture-phase listeners for the leader and follow-ups", () => {
    // Regression guard: stopImmediatePropagation must prevent sibling document
    // capture-phase listeners (e.g. terminal-init's Cmd+C handler) from seeing
    // keys that the chord engine fully owns.
    const { bindings } = makeBindings();
    renderHook(() => useKeyboardChords(bindings));

    // Register the sibling spy AFTER the hook mounts so it is a later sibling —
    // the realistic scenario where terminal-init also registered on capture.
    const siblingSpy = vi.fn();
    document.addEventListener("keydown", siblingSpy, true);

    try {
      // 1. Leader: sibling must NOT see it.
      act(() => {
        dispatchLeader();
      });
      expect(siblingSpy).not.toHaveBeenCalled();

      // 2. Follow-up while pending: sibling must NOT see it either.
      act(() => {
        dispatchKey("n");
      });
      expect(siblingSpy).not.toHaveBeenCalled();
    } finally {
      document.removeEventListener("keydown", siblingSpy, true);
    }
  });

  it("removes listeners and clears the timer on unmount", () => {
    vi.useFakeTimers();
    const { bindings, n } = makeBindings();
    const removeSpy = vi.spyOn(document, "removeEventListener");
    const windowRemoveSpy = vi.spyOn(window, "removeEventListener");
    const { unmount } = renderHook(() => useKeyboardChords(bindings));

    act(() => {
      dispatchLeader();
    });

    unmount();
    expect(removeSpy).toHaveBeenCalledWith("keydown", expect.any(Function), true);
    expect(windowRemoveSpy).toHaveBeenCalledWith("blur", expect.any(Function));

    // A leader after unmount does nothing (listener gone), and the pending
    // timer is cleared so no late callback fires.
    act(() => {
      dispatchLeader();
    });
    act(() => {
      vi.advanceTimersByTime(2000);
    });
    expect(n).not.toHaveBeenCalled();
  });
});
