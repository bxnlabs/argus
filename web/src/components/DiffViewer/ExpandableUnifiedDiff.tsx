import { memo, useEffect, useMemo, useRef } from "react";
import type { ParsedDiff, DiffHunk } from "@/lib/diff-parser";
import type { ReviewComment, DiffPosition } from "@/types";
import { useExpandableDiff, type ExpansionContext } from "@/hooks/useExpandableDiff";
import { UnifiedDiff } from "./UnifiedDiff";

/** Context window expanded around a caseB anchor on mount. */
const AUTO_EXPAND_RADIUS = 3;

interface ExpandableUnifiedDiffProps {
  diff: ParsedDiff;
  totalLines: number;
  expansionContext: ExpansionContext;
  fileName: string;
  expanded?: boolean;
  onToggle?: () => void;
  wrapLines?: boolean;
  comments?: ReviewComment[];
  activeCommentLine?: { position: DiffPosition } | null;
  onLineClick?: (position: DiffPosition) => void;
  onAddComment?: (body: string) => void;
  onCancelComment?: () => void;
  onDeleteComment?: (id: string) => void;
  onEditComment?: (id: string, body: string) => void;
  onEditCommentRequest?: (comment: ReviewComment) => void;
  onCommentRef?: (id: string, el: HTMLElement | null) => void;
  onExpandedHunksChange?: (hunks: DiffHunk[]) => void;
  /**
   * New-side line numbers to auto-expand context around on first mount.
   * Used to surface caseB comments — comments whose anchor is within the
   * file's line range but not yet covered by any hunk. Each anchor fires
   * exactly once per file lifecycle (tracked by a ref keyed on the line
   * number), so a user who manually collapses an expanded region won't see
   * it re-expand on re-render.
   */
  autoExpandLines?: number[];
}

export const ExpandableUnifiedDiff = memo(function ExpandableUnifiedDiff({
  diff,
  totalLines: totalLinesProp,
  expansionContext,
  onExpandedHunksChange,
  autoExpandLines,
  ...unifiedDiffProps
}: ExpandableUnifiedDiffProps) {
  const { hunks, totalLines, expandLoading, expandErrors, handleExpand, expandToLine } =
    useExpandableDiff(diff.hunks, totalLinesProp, expansionContext);

  // Report expanded hunks to parent for comment/snippet resolution
  useEffect(() => {
    onExpandedHunksChange?.(hunks);
  }, [hunks, onExpandedHunksChange]);

  // Track anchors that have already been auto-expanded so this only fires
  // once per (file, anchor) pair, not on every render. Reset when the
  // underlying diff changes (new compare data).
  const firedAnchorsRef = useRef<Set<number>>(new Set());
  useEffect(() => {
    firedAnchorsRef.current = new Set();
  }, [diff]);
  useEffect(() => {
    if (!autoExpandLines || autoExpandLines.length === 0) return;
    for (const line of autoExpandLines) {
      if (firedAnchorsRef.current.has(line)) continue;
      firedAnchorsRef.current.add(line);
      void expandToLine(line, AUTO_EXPAND_RADIUS);
    }
  }, [autoExpandLines, expandToLine]);

  // Stable diff object — only changes when the underlying diff or expanded hunks change
  const expandedDiff = useMemo<ParsedDiff>(
    () => ({ ...diff, hunks }),
    [diff, hunks],
  );

  return (
    <UnifiedDiff
      {...unifiedDiffProps}
      diff={expandedDiff}
      onExpand={handleExpand}
      expandLoading={expandLoading}
      expandErrors={expandErrors}
      totalLines={totalLines}
    />
  );
});
