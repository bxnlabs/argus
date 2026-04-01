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
  | { type: "RESET"; hunks: DiffHunk[]; totalLines: number };

interface ExpandableDiffState {
  hunks: DiffHunk[];
  totalLines: number;
  generation: number;
}

const EXPAND_INCREMENT = 20;

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
      };

    case "EXPAND_UP": {
      const { hunkIndex, lines } = action;
      if (hunkIndex < 0 || hunkIndex >= state.hunks.length || lines.length === 0) return state;

      const hunks = [...state.hunks];
      const hunk = { ...hunks[hunkIndex] };

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

      return { hunks, totalLines: state.totalLines, generation: state.generation + 1 };
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

      return { hunks, totalLines: state.totalLines, generation: state.generation + 1 };
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
  });

  const generationRef = useRef(state.generation);
  generationRef.current = state.generation;

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

  const resetHunks = useCallback((hunks: DiffHunk[], newTotalLines: number) => {
    abortAll();
    dispatch({ type: "RESET", hunks, totalLines: newTotalLines });
  }, [abortAll]);

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
      } else {
        const hunk = hunks[hunkIndex];
        if (!hunk) return;
        start = hunk.newStart + hunk.newCount;
        end = Math.min(state.totalLines, start + EXPAND_INCREMENT - 1);
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
    resetHunks,
    abortAll,
  };
}
