import { describe, it, expect, afterEach, vi } from "vitest";
import { renderHook, act, cleanup } from "@testing-library/react";
import { useSingleFlight } from "./useSingleFlight";

// Not automatic in this project (no `globals: true`).
afterEach(cleanup);

// A promise plus its resolve/reject, so a test can hold a call open and settle
// it on demand.
function deferred() {
  let resolve!: () => void;
  let reject!: (err: unknown) => void;
  const promise = new Promise<void>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

describe("useSingleFlight", () => {
  it("marks itself pending in the same commit as the dispatch", () => {
    const { result } = renderHook(() => useSingleFlight());
    expect(result.current.pending).toBe(false);

    act(() => result.current.run(() => deferred().promise));

    expect(result.current.pending).toBe(true);
  });

  it("ignores a second dispatch in the same tick", () => {
    const { result } = renderHook(() => useSingleFlight());
    const fn = vi.fn(() => deferred().promise);

    // Both calls before React re-renders — the whole reason the lock is a ref
    // and not the `pending` state.
    act(() => {
      result.current.run(fn);
      result.current.run(fn);
    });

    expect(fn).toHaveBeenCalledTimes(1);
  });

  it("releases once the call settles, on success and on failure", async () => {
    const { result } = renderHook(() => useSingleFlight());

    const ok = deferred();
    act(() => result.current.run(() => ok.promise));
    await act(async () => {
      ok.resolve();
      await ok.promise;
    });
    expect(result.current.pending).toBe(false);

    const bad = deferred();
    bad.promise.catch(() => {});
    act(() => result.current.run(() => bad.promise));
    expect(result.current.pending).toBe(true);
    await act(async () => {
      bad.reject(new Error("nope"));
      await bad.promise.catch(() => {});
    });
    expect(result.current.pending).toBe(false);
  });

  it("takes the lock again once the previous call has settled", async () => {
    const { result } = renderHook(() => useSingleFlight());
    const fn = vi.fn(() => Promise.resolve());

    await act(async () => result.current.run(fn));
    await act(async () => result.current.run(fn));

    expect(fn).toHaveBeenCalledTimes(2);
  });

  it("releases a call that settles before any pending snapshot is observed", async () => {
    const { result } = renderHook(() => useSingleFlight());

    // An already-rejected promise: dispatch and settle land in the same task,
    // so a release keyed on watching `pending` fall would never fire.
    await act(async () => {
      result.current.run(() => Promise.reject(new Error("immediate")));
    });

    expect(result.current.pending).toBe(false);
  });

  it("releases and rethrows when the call throws synchronously", () => {
    const { result } = renderHook(() => useSingleFlight());

    expect(() =>
      act(() =>
        result.current.run(() => {
          throw new Error("sync");
        }),
      ),
    ).toThrow("sync");
    expect(result.current.pending).toBe(false);
  });

  it("drops the lock on reset so the next dispatch is accepted", () => {
    const { result } = renderHook(() => useSingleFlight());
    const fn = vi.fn(() => deferred().promise);

    act(() => result.current.run(fn));
    act(() => result.current.reset());
    expect(result.current.pending).toBe(false);

    act(() => result.current.run(fn));
    expect(fn).toHaveBeenCalledTimes(2);
    expect(result.current.pending).toBe(true);
  });

  it("does not let a disowned call release a newer one's lock", async () => {
    const { result } = renderHook(() => useSingleFlight());

    const first = deferred();
    act(() => result.current.run(() => first.promise));
    // Reopened/retargeted while the first call is still running.
    act(() => result.current.reset());
    const second = deferred();
    act(() => result.current.run(() => second.promise));

    await act(async () => {
      first.resolve();
      await first.promise;
    });

    // The first call settling must not unlock the second.
    expect(result.current.pending).toBe(true);
    const fn = vi.fn(() => deferred().promise);
    act(() => result.current.run(fn));
    expect(fn).not.toHaveBeenCalled();
  });
});
