import { useReducer, useCallback, useRef, useEffect } from "react";
import type { DiffHunk, DiffLine } from "@/lib/diff-parser";
import { fetchFileLines } from "@/data/git/file-lines";
import { ApiError } from "@/api/client";

// --- Types ---

export interface ExpansionContext {
  repoPath: string;
  filePath: string;
  ref?: string;
}

export type ExpandDirection = "up" | "down";

type ExpandAction =
  | { type: "EXPAND_UP"; hunkIndex: number; lines: DiffLine[] }
  | { type: "EXPAND_DOWN"; hunkIndex: number; lines: DiffLine[] }
  | { type: "INSERT_HUNK"; hunk: DiffHunk }
  | { type: "RESET"; hunks: DiffHunk[]; totalLines: number };

interface ExpandableDiffState {
  hunks: DiffHunk[];
  totalLines: number;
  // Bumped on every mutation. Manual expand fetches capture it and bail if any
  // intervening action ran, since their captured hunkIndex would be stale.
  generation: number;
  // Bumped only on RESET (new diff data). Auto-expand fetches capture this
  // instead of `generation` so that concurrent sibling inserts don't invalidate
  // one another — only a genuine diff reset should discard an in-flight expand.
  resetGeneration: number;
}

const EXPAND_INCREMENT = 10;

// --- Helpers ---

/**
 * Derives the old-side line number for a context line given its new-side number
 * and the hunk whose boundary determines the offset.
 *
 * Offset formula: oldOffset = (hunk.oldStart + hunk.oldCount) - (hunk.newStart + hunk.newCount)
 * For context lines: oldLineNumber = newLineNumber + oldOffset
 */
function computeOldOffset(hunk: DiffHunk): number {
  return (hunk.oldStart + hunk.oldCount) - (hunk.newStart + hunk.newCount);
}

/**
 * Computes the old-to-new line offset at a given old-side line number
 * by finding the nearest preceding hunk boundary.
 *
 * offset = (hunk.oldStart + hunk.oldCount) - (hunk.newStart + hunk.newCount)
 * newLineNumber = oldLineNumber - offset
 */
export function computeOldToNewOffset(oldLine: number, hunks: DiffHunk[]): number {
  let offset = 0;
  for (const h of hunks) {
    if (h.oldStart + h.oldCount <= oldLine) {
      offset = computeOldOffset(h);
    }
  }
  return offset;
}

function makeContextLine(content: string, newLineNum: number, oldOffset: number): DiffLine {
  return {
    type: "context",
    content,
    newLineNumber: newLineNum,
    oldLineNumber: newLineNum + oldOffset,
  };
}

function reconstructHeader(oldStart: number, oldCount: number, newStart: number, newCount: number): string {
  return `@@ -${oldStart},${oldCount} +${newStart},${newCount} @@`;
}

// --- Reducer ---

export function expandableDiffReducer(state: ExpandableDiffState, action: ExpandAction): ExpandableDiffState {
  switch (action.type) {
    case "RESET":
      return {
        hunks: action.hunks,
        totalLines: action.totalLines,
        generation: state.generation + 1,
        resetGeneration: state.resetGeneration + 1,
      };

    case "EXPAND_UP": {
      const { hunkIndex, lines } = action;
      if (hunkIndex < 0 || hunkIndex >= state.hunks.length || lines.length === 0) return state;

      const hunks = [...state.hunks];
      const hunk = { ...hunks[hunkIndex] };
      // Preserve the original stableKey through expansion

      // Offset: use previous hunk's end boundary, or 0 if first hunk
      const oldOffset = hunkIndex > 0 ? computeOldOffset(hunks[hunkIndex - 1]) : 0;

      const newLines: DiffLine[] = lines.map((l, i) => {
        const newLineNum = hunk.newStart - lines.length + i;
        return makeContextLine(l.content, newLineNum, oldOffset);
      });

      hunk.lines = [...newLines, ...hunk.lines];
      hunk.newStart -= lines.length;
      hunk.newCount += lines.length;
      hunk.oldStart -= lines.length;
      hunk.oldCount += lines.length;
      hunk.header = reconstructHeader(hunk.oldStart, hunk.oldCount, hunk.newStart, hunk.newCount);
      hunks[hunkIndex] = hunk;

      return { ...state, hunks, generation: state.generation + 1 };
    }

    case "EXPAND_DOWN": {
      const { hunkIndex, lines } = action;
      if (hunkIndex < 0 || hunkIndex >= state.hunks.length || lines.length === 0) return state;

      const hunks = [...state.hunks];
      const hunk = { ...hunks[hunkIndex] };

      const oldOffset = computeOldOffset(hunk);
      const startNewLine = hunk.newStart + hunk.newCount;

      const newLines: DiffLine[] = lines.map((l, i) =>
        makeContextLine(l.content, startNewLine + i, oldOffset),
      );

      hunk.lines = [...hunk.lines, ...newLines];
      hunk.newCount += lines.length;
      hunk.oldCount += lines.length;
      hunk.header = reconstructHeader(hunk.oldStart, hunk.oldCount, hunk.newStart, hunk.newCount);
      hunks[hunkIndex] = hunk;

      return { ...state, hunks, generation: state.generation + 1 };
    }

    case "INSERT_HUNK": {
      const { hunk } = action;
      const newEnd = hunk.newStart + hunk.newCount - 1;
      // Skip when the new-side range overlaps an existing hunk: either the
      // anchor is already covered, or a concurrent insert beat us here.
      // Upstream coalescing keeps sibling synthetic hunks from overlapping, so
      // this only fires for the already-covered / race case.
      const overlaps = state.hunks.some((h) => {
        const hEnd = h.newStart + h.newCount - 1;
        return hunk.newStart <= hEnd && h.newStart <= newEnd;
      });
      if (overlaps) return state;
      // Insert at the sorted position computed from the CURRENT state, so a
      // concurrent insert (which captured an earlier hunk snapshot) still lands
      // in the right place rather than at a now-stale index.
      let pos = state.hunks.findIndex((h) => h.newStart > hunk.newStart);
      if (pos === -1) pos = state.hunks.length;
      const hunks = [...state.hunks.slice(0, pos), hunk, ...state.hunks.slice(pos)];
      return { ...state, hunks, generation: state.generation + 1 };
    }

    default:
      return state;
  }
}

// --- Hook ---

export function useExpandableDiff(
  initialHunks: DiffHunk[],
  totalLines: number,
  ctx: ExpansionContext,
) {
  const [state, dispatch] = useReducer(expandableDiffReducer, {
    hunks: initialHunks,
    totalLines,
    generation: 0,
    resetGeneration: 0,
  });

  const generationRef = useRef(state.generation);
  generationRef.current = state.generation;

  const resetGenerationRef = useRef(state.resetGeneration);
  resetGenerationRef.current = state.resetGeneration;

  // Track AbortControllers for in-flight requests
  const abortControllersRef = useRef<Set<AbortController>>(new Set());

  // Loading state keyed by "direction-hunkIndex"
  const [, forceUpdate] = useReducer((x: number) => x + 1, 0);
  const loadingRef = useRef<Record<string, boolean>>({});

  // Error state keyed by "direction-hunkIndex": "permanent" (404/413/422) or "transient" (500/network)
  const errorRef = useRef<Record<string, "permanent" | "transient">>({});

  // Reset when initialHunks change (new diff data)
  const prevHunksRef = useRef(initialHunks);
  const prevTotalLinesRef = useRef(totalLines);
  useEffect(() => {
    if (prevHunksRef.current !== initialHunks || prevTotalLinesRef.current !== totalLines) {
      prevHunksRef.current = initialHunks;
      prevTotalLinesRef.current = totalLines;
      dispatch({ type: "RESET", hunks: initialHunks, totalLines });
      // Cancel all in-flight requests
      for (const controller of abortControllersRef.current) {
        controller.abort();
      }
      abortControllersRef.current.clear();
      loadingRef.current = {};
      errorRef.current = {};
    }
  }, [initialHunks, totalLines]);

  const abortAll = useCallback(() => {
    for (const controller of abortControllersRef.current) {
      controller.abort();
    }
    abortControllersRef.current.clear();
    loadingRef.current = {};
    errorRef.current = {};
    forceUpdate();
  }, []);

  // Abort in-flight requests on unmount (e.g. a diff lazy-unmounted when
  // scrolled out of view) so a late response can't dispatch into a torn-down
  // reducer or drive a stale fallback. No forceUpdate here — the component is
  // gone, and a state update during teardown would warn.
  useEffect(() => {
    const controllers = abortControllersRef.current;
    return () => {
      for (const controller of controllers) {
        controller.abort();
      }
      controllers.clear();
    };
  }, []);

  const resetHunks = useCallback((hunks: DiffHunk[], newTotalLines: number) => {
    abortAll();
    dispatch({ type: "RESET", hunks, totalLines: newTotalLines });
  }, [abortAll]);

  /**
   * Inserts a synthetic context hunk covering `newLine` ± `radius` so a
   * caseB comment (file is in compare, line is within file range, but the
   * anchor isn't in any hunk yet) renders inline.
   *
   * The synthetic hunk's line numbers are derived from the actual fetched range
   * (not from a neighboring hunk boundary as EXPAND_UP/DOWN does). The old-side
   * line numbers are computed via the cumulative offset of preceding hunks so
   * context lines carry both numbers and L-side comments can match against them.
   * The reducer places it at the correct sorted position and drops it if it
   * would overlap an existing hunk.
   *
   * Returns whether the anchor ended up covered, so the caller can fall back to
   * rendering the comment in the unanchored section when it didn't:
   *   - `true`  — the anchor is now (or already was) inside a hunk, or the
   *     request was superseded by a diff reset that will re-fire the expand.
   *   - `false` — the context could not be fetched (EOF, empty range, or a
   *     fetch error), so nothing was inserted and the comment has no inline home.
   */
  const expandToLine = useCallback(
    async (newLine: number, radius: number): Promise<boolean> => {
      const hunks = state.hunks;

      // Clamp the requested range against neighboring hunks so the synthetic
      // hunk doesn't overlap them.
      let lowerBound = 1;
      let upperBound = Math.max(state.totalLines, newLine + radius);

      for (let i = 0; i < hunks.length; i++) {
        const h = hunks[i];
        const hStart = h.newStart;
        const hEnd = h.newStart + h.newCount - 1;
        if (newLine >= hStart && newLine <= hEnd) {
          // Anchor is already inside an existing hunk — already covered.
          return true;
        }
        if (newLine < hStart) {
          upperBound = Math.min(upperBound, hStart - 1);
          break;
        }
        lowerBound = Math.max(lowerBound, hEnd + 1);
      }

      const start = Math.max(lowerBound, newLine - radius);
      const end = Math.min(upperBound, newLine + radius);
      if (start > end) return false;

      const loadingKey = `auto-${newLine}`;
      if (errorRef.current[loadingKey] === "permanent") return false;
      delete errorRef.current[loadingKey];

      const capturedReset = resetGenerationRef.current;
      const controller = new AbortController();
      abortControllersRef.current.add(controller);
      loadingRef.current[loadingKey] = true;
      forceUpdate();

      try {
        const result = await fetchFileLines({
          path: ctx.repoPath,
          file: ctx.filePath,
          start,
          end,
          ref: ctx.ref,
          signal: controller.signal,
        });

        // Only a diff reset (new compare data) invalidates an in-flight auto
        // expand; sibling inserts must not. The reset path re-fires the expand,
        // so report it as handled rather than failed.
        if (resetGenerationRef.current !== capturedReset) return true;
        if (result.lines.length === 0) return false;

        // Compute old-side line number for `result.start` by accumulating
        // the cumulative offset of hunks whose end precedes it. This mirrors
        // computeOldToNewOffset's logic but applied at result.start.
        let cumulativeOffset = 0;
        for (const h of hunks) {
          if (h.newStart + h.newCount <= result.start) {
            cumulativeOffset = computeOldOffset(h);
          }
        }

        const diffLines: DiffLine[] = result.lines.map((content, i) =>
          makeContextLine(content, result.start + i, cumulativeOffset),
        );

        const newStart = result.start;
        const newCount = result.lines.length;
        const oldStart = newStart + cumulativeOffset;
        const oldCount = newCount;

        const syntheticHunk: DiffHunk = {
          header: reconstructHeader(oldStart, oldCount, newStart, newCount),
          oldStart,
          oldCount,
          newStart,
          newCount,
          lines: diffLines,
          stableKey: `auto-${newStart}-${newCount}`,
        };

        dispatch({ type: "INSERT_HUNK", hunk: syntheticHunk });
        return true;
      } catch (err) {
        if (err instanceof DOMException && err.name === "AbortError") {
          // Aborted by a reset/unmount — the reset path re-fires; treat as handled.
          return true;
        } else if (err instanceof ApiError) {
          if (err.status === 404 || err.status === 413 || err.status === 422) {
            errorRef.current[loadingKey] = "permanent";
          } else {
            errorRef.current[loadingKey] = "transient";
          }
        } else {
          errorRef.current[loadingKey] = "transient";
        }
        return false;
      } finally {
        abortControllersRef.current.delete(controller);
        delete loadingRef.current[loadingKey];
        forceUpdate();
      }
    },
    [state.hunks, state.totalLines, ctx.repoPath, ctx.filePath, ctx.ref],
  );

  const handleExpand = useCallback(
    async (direction: ExpandDirection, hunkIndex: number) => {
      const loadingKey = `${direction}-${hunkIndex}`;

      // Skip if this expander is permanently disabled
      if (errorRef.current[loadingKey] === "permanent") return;

      // Clear any previous transient error on retry
      delete errorRef.current[loadingKey];

      const hunks = state.hunks;
      let start: number;
      let end: number;

      if (direction === "up") {
        const hunk = hunks[hunkIndex];
        if (!hunk) return;
        end = hunk.newStart - 1;
        start = Math.max(1, end - EXPAND_INCREMENT + 1);
        // Clamp to previous hunk's end to prevent overlap
        if (hunkIndex > 0) {
          const prev = hunks[hunkIndex - 1];
          const prevEnd = prev.newStart + prev.newCount;
          start = Math.max(start, prevEnd);
        }
      } else {
        const hunk = hunks[hunkIndex];
        if (!hunk) return;
        start = hunk.newStart + hunk.newCount;
        end = Math.min(state.totalLines, start + EXPAND_INCREMENT - 1);
        // Clamp to next hunk's start to prevent overlap
        if (hunkIndex < hunks.length - 1) {
          const next = hunks[hunkIndex + 1];
          end = Math.min(end, next.newStart - 1);
        }
      }

      if (start > end) return;

      const capturedGeneration = generationRef.current;

      const controller = new AbortController();
      abortControllersRef.current.add(controller);
      loadingRef.current[loadingKey] = true;
      forceUpdate();

      try {
        const result = await fetchFileLines({
          path: ctx.repoPath,
          file: ctx.filePath,
          start,
          end,
          ref: ctx.ref,
          signal: controller.signal,
        });

        // Stale response rejection
        if (generationRef.current !== capturedGeneration) return;

        // Determine old offset for line number derivation
        const offsetHunk = direction === "up"
          ? (hunkIndex > 0 ? hunks[hunkIndex - 1] : null)
          : hunks[hunkIndex];
        const oldOffset = offsetHunk ? computeOldOffset(offsetHunk) : 0;

        const diffLines: DiffLine[] = result.lines.map((content, i) =>
          makeContextLine(content, result.start + i, oldOffset),
        );

        const actionType = direction === "up" ? "EXPAND_UP" as const : "EXPAND_DOWN" as const;
        dispatch({ type: actionType, hunkIndex, lines: diffLines });
      } catch (err) {
        // Aborted requests are expected during cleanup — ignore silently
        if (err instanceof DOMException && err.name === "AbortError") {
          // no-op
        } else if (err instanceof ApiError) {
          // Permanent failures: file not found, too large, or binary — disable this expander
          if (err.status === 404 || err.status === 413 || err.status === 422) {
            errorRef.current[loadingKey] = "permanent";
          } else {
            errorRef.current[loadingKey] = "transient";
          }
        } else {
          errorRef.current[loadingKey] = "transient";
        }
      } finally {
        abortControllersRef.current.delete(controller);
        delete loadingRef.current[loadingKey];
        forceUpdate();
      }
    },
    [state.hunks, state.totalLines, ctx.repoPath, ctx.filePath, ctx.ref],
  );

  return {
    hunks: state.hunks,
    totalLines: state.totalLines,
    generation: state.generation,
    expandLoading: loadingRef.current,
    expandErrors: errorRef.current,
    handleExpand,
    expandToLine,
    resetHunks,
    abortAll,
    dispatch,
  };
}
