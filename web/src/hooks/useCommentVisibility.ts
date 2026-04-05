import { useCallback, useRef } from "react";
import type { DiffHunk, DiffLine } from "@/lib/diff-parser";
import type { DiffPosition, ReviewComment } from "@/types";
import { fetchFileLines } from "@/data/git/file-lines";
import { computeOldToNewOffset } from "@/hooks/useExpandableDiff";

const CONTEXT_WINDOW = 3;

interface CommentVisibilityOptions {
  repoPath: string;
  filePath: string;
  headRef?: string;
  baseRef?: string;
  hunks: DiffHunk[];
  onInsertSynthetic: (hunk: DiffHunk, insertIndex: number) => void;
  onCommentStatusChange?: (commentId: string, status: "context_unavailable") => void;
}

function isPositionVisible(hunks: DiffHunk[], position: DiffPosition): boolean {
  for (const hunk of hunks) {
    for (const line of hunk.lines) {
      if (position.side === "L" && line.oldLineNumber === position.line) return true;
      if (position.side === "R" && line.newLineNumber === position.line) return true;
    }
  }
  return false;
}

function findInsertIndex(hunks: DiffHunk[], lineNum: number, side: DiffPosition["side"]): number {
  for (let i = 0; i < hunks.length; i++) {
    const hunkLine = side === "L" ? hunks[i].oldStart : hunks[i].newStart;
    if (hunkLine > lineNum) return i;
  }
  return hunks.length;
}

export function useCommentVisibility(options: CommentVisibilityOptions) {
  const pendingRef = useRef<Set<string>>(new Set());

  const ensureCommentVisible = useCallback(async (comment: ReviewComment): Promise<void> => {
    const pos = comment.line.to;
    if (isPositionVisible(options.hunks, pos)) return;

    const key = `${pos.side}${pos.line}`;
    if (pendingRef.current.has(key)) return;
    pendingRef.current.add(key);

    const ref = pos.side === "L" ? options.baseRef : options.headRef;
    const file = pos.side === "L" && comment.oldPath ? comment.oldPath : comment.file;
    const start = Math.max(1, pos.line - CONTEXT_WINDOW);
    const end = pos.line + CONTEXT_WINDOW;

    try {
      const result = await fetchFileLines({
        path: options.repoPath,
        file,
        start,
        end,
        ref,
      });

      // Derive side-aware coordinates using hunk boundary offsets
      const offset = pos.side === "L" ? computeOldToNewOffset(start, options.hunks) : 0;
      const oldStart = pos.side === "L" ? start : start + offset;
      const newStart = pos.side === "L" ? start - offset : start;

      const lines: DiffLine[] = result.lines.map((content, i) => ({
        type: "context" as const,
        content,
        newLineNumber: newStart + i,
        oldLineNumber: oldStart + i,
      }));

      const syntheticHunk: DiffHunk = {
        header: `@@ -${oldStart},${lines.length} +${newStart},${lines.length} @@`,
        oldStart,
        oldCount: lines.length,
        newStart,
        newCount: lines.length,
        lines,
      };

      const insertIdx = findInsertIndex(options.hunks, start, pos.side);
      options.onInsertSynthetic(syntheticHunk, insertIdx);
    } catch {
      options.onCommentStatusChange?.(comment.id, "context_unavailable");
    } finally {
      pendingRef.current.delete(key);
    }
  }, [options]);

  const ensureAllVisible = useCallback(async (comments: ReviewComment[]) => {
    const hidden = comments.filter((c) => !isPositionVisible(options.hunks, c.line.to));
    await Promise.all(hidden.map((c) => ensureCommentVisible(c)));
  }, [options.hunks, ensureCommentVisible]);

  return { ensureCommentVisible, ensureAllVisible };
}
