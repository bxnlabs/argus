import { useLayoutEffect, useRef, type RefObject } from "react";

interface ScrollToFileCorrectionParams {
  /** The scroll pane the target is aligned within. */
  scrollContainerRef: RefObject<HTMLElement | null>;
  /** Resolves the wrapper element for a path. Must be stable (useCallback). */
  getTarget: (path: string) => HTMLElement | null;
  selectedPath: string | null;
  isMobile: boolean;
  mobileShowDiffs: boolean;
  /** Bumped per scroll-to-file request; re-runs the correction window. */
  requestId: number;
  /** How long to keep correcting after the request, in ms. */
  windowMs?: number;
}

/**
 * After a scroll-to-file request, re-aligns the selected file's wrapper to the
 * top of the scroll pane by measured delta on every animation frame for a short
 * window.
 *
 * Placeholder heights are only approximate, so lazy files mounting and resizing
 * after the scroll would otherwise drift the target — and browsers without CSS
 * scroll anchoring (iOS Safari) wouldn't correct it. Tracking the target's
 * *position* each frame fixes drift even when total content height is unchanged,
 * and composes with native anchoring (a no-op when it already held position).
 *
 * Yields immediately on real user scroll input (programmatic scrollTop writes
 * don't dispatch wheel/touch/key). keydown is observed on window because focus
 * is usually on the sidebar row — outside the pane — right after a file click.
 */
export function useScrollToFileCorrection({
  scrollContainerRef,
  getTarget,
  selectedPath,
  isMobile,
  mobileShowDiffs,
  requestId,
  windowMs = 400,
}: ScrollToFileCorrectionParams): void {
  const ridRef = useRef(0);

  useLayoutEffect(() => {
    if (!selectedPath) return;
    if (isMobile && !mobileShowDiffs) return;
    const pane = scrollContainerRef.current;
    if (!pane) return;
    const target = getTarget(selectedPath);
    if (!target) return;

    const rid = ++ridRef.current;
    let active = true;
    let rafHandle = 0;
    let timeoutHandle = 0;

    const align = () => {
      // Skip if superseded, torn down, or the target detached — e.g. a compare
      // swap replaced the diff nodes mid-window. A detached node measures 0,0
      // and would otherwise trigger a spurious scroll.
      if (!active || rid !== ridRef.current || !target.isConnected) return;
      const delta =
        target.getBoundingClientRect().top - pane.getBoundingClientRect().top;
      if (Math.abs(delta) > 1) pane.scrollTop += delta;
    };

    const deadline = performance.now() + windowMs;
    const tick = () => {
      align();
      if (active && performance.now() < deadline) {
        rafHandle = requestAnimationFrame(tick);
      }
    };

    const teardown = () => {
      if (!active) return;
      active = false;
      cancelAnimationFrame(rafHandle);
      clearTimeout(timeoutHandle);
      pane.removeEventListener("wheel", teardown);
      pane.removeEventListener("touchstart", teardown);
      window.removeEventListener("keydown", teardown);
    };

    rafHandle = requestAnimationFrame(tick);
    pane.addEventListener("wheel", teardown, { passive: true });
    pane.addEventListener("touchstart", teardown, { passive: true });
    window.addEventListener("keydown", teardown);
    timeoutHandle = window.setTimeout(teardown, windowMs);

    return teardown;
  }, [
    requestId,
    selectedPath,
    isMobile,
    mobileShowDiffs,
    getTarget,
    scrollContainerRef,
    windowMs,
  ]);
}
