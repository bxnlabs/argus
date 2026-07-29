import { useCallback, useRef, useState } from "react";

export interface SingleFlight {
  // Whether a call dispatched through `run` is still outstanding. A render
  // mirror of the lock: the ref is what serialises, but a ref change does not
  // re-render, so without this the form stays visually live for the whole task
  // it takes the mutation-cache snapshot to arrive — locked but not looking it.
  pending: boolean;
  // Dispatch `fn` unless a call is already in flight. Releases on settle
  // (success or failure); errors stay the caller's.
  run: (fn: () => void | Promise<void>) => void;
  // Drop the lock and disown whatever is in flight.
  reset: () => void;
}

/**
 * Serialises one dialog action: at most one call outstanding, with a render
 * mirror so the form can disable itself in the same commit as the dispatch.
 *
 * Shared by the dialogs that dispatch a session mutation (create, change
 * profile) because all of them need the same three things, and each one needs
 * them for a reason that is not obvious:
 *
 * - A *synchronous* lock. Pending state derived from TanStack's mutation cache
 *   reaches a component asynchronously — TanStack recomputes its snapshot on
 *   each cache event but schedules React's re-read — so it cannot lock out a
 *   second dispatch in the same tick, and every one of these dialogs has two
 *   entry points (the button and the ⌘/Ctrl+Enter handler).
 *
 * - Release driven by the call settling, not by watching pending state fall. A
 *   call that settles before TanStack's scheduled notify reaches React never
 *   renders a pending snapshot at all (an immediately-rejecting fetch
 *   dispatches pending then error in the same task), so a release keyed on that
 *   transition would strand the form.
 *
 * - A generation, so a settle can only release the lock it took. These dialogs
 *   outlive their own call — reopened for the next session, or retargeted at
 *   another one, while the first is still running — and an ungated release
 *   would then clear a *newer* call's lock. `reset` bumps the generation for
 *   exactly that reason: it disowns the in-flight call rather than waiting for
 *   it.
 *
 * This owns the *lock* only. Whether a given pending mutation is the one this
 * form started is separate — see `submitted` in NewSessionDialog, where
 * `isCreating` is node-wide rather than per-form.
 */
export function useSingleFlight(): SingleFlight {
  const lockRef = useRef(false);
  const generationRef = useRef(0);
  const [pending, setPending] = useState(false);

  const reset = useCallback(() => {
    generationRef.current += 1;
    lockRef.current = false;
    setPending(false);
  }, []);

  const run = useCallback((fn: () => void | Promise<void>) => {
    if (lockRef.current) return;
    const generation = ++generationRef.current;
    lockRef.current = true;
    setPending(true);
    const release = () => {
      if (generationRef.current !== generation) return;
      lockRef.current = false;
      setPending(false);
    };
    try {
      Promise.resolve(fn()).then(release, release);
    } catch (err) {
      // A synchronous throw escapes the promise chain, and would strand the
      // lock the same way an unobserved settle does. Release, then let it
      // propagate as it would have.
      release();
      throw err;
    }
  }, []);

  return { pending, run, reset };
}
