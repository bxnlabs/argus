import { memo, useEffect, useMemo, useRef } from "react";
import type { ParsedDiff, DiffHunk } from "@/lib/diff-parser";
import type { ReviewComment, DiffPosition } from "@/types";
import type { AutoExpandTarget } from "@/lib/compare-comments";
import { useExpandableDiff, type ExpansionContext } from "@/hooks/useExpandableDiff";
import { UnifiedDiff } from "./UnifiedDiff";

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
   * Coalesced context windows to auto-expand on first mount, surfacing caseB
   * comments — comments whose anchor is within the file's line range but not
   * yet covered by any hunk. Nearby anchors are merged upstream so their
   * windows never overlap. Each target fires exactly once per file lifecycle
   * (tracked by a ref keyed on the center line), so a user who manually
   * collapses an expanded region won't see it re-expand on re-render.
   */
  autoExpandTargets?: AutoExpandTarget[];
  /**
   * Called with the affected comment IDs when a target's auto-expansion fails
   * to cover its anchor (EOF/empty range/fetch error), so the parent can fall
   * back to rendering those comments in the unanchored section.
   */
  onAutoExpandFailed?: (commentIds: string[]) => void;
}

export const ExpandableUnifiedDiff = memo(function ExpandableUnifiedDiff({
  diff,
  totalLines: totalLinesProp,
  expansionContext,
  onExpandedHunksChange,
  autoExpandTargets,
  onAutoExpandFailed,
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
    if (!autoExpandTargets || autoExpandTargets.length === 0) return;
    for (const t of autoExpandTargets) {
      if (firedAnchorsRef.current.has(t.line)) continue;
      firedAnchorsRef.current.add(t.line);
      void expandToLine(t.line, t.radius).then((covered) => {
        if (!covered) onAutoExpandFailed?.(t.commentIds);
      });
    }
  }, [autoExpandTargets, expandToLine, onAutoExpandFailed]);

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
