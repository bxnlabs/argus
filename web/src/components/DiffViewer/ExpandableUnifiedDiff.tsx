import { memo, useCallback, useEffect } from "react";
import type { ParsedDiff, DiffHunk } from "@/lib/diff-parser";
import type { ReviewComment, DiffPosition } from "@/types";
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
  onRegisterInsertSynthetic?: (handler: (hunk: DiffHunk, insertIndex: number) => void) => void;
}

export const ExpandableUnifiedDiff = memo(function ExpandableUnifiedDiff({
  diff,
  totalLines: totalLinesProp,
  expansionContext,
  onExpandedHunksChange,
  onRegisterInsertSynthetic,
  ...unifiedDiffProps
}: ExpandableUnifiedDiffProps) {
  const { hunks, totalLines, expandLoading, expandErrors, handleExpand, dispatch } = useExpandableDiff(
    diff.hunks,
    totalLinesProp,
    expansionContext,
  );

  // Report expanded hunks to parent for comment/snippet resolution
  useEffect(() => {
    onExpandedHunksChange?.(hunks);
  }, [hunks, onExpandedHunksChange]);

  // Register a stable synthetic-hunk insertion handler with the parent
  const handleInsertSynthetic = useCallback((hunk: DiffHunk, insertIndex: number) => {
    dispatch({ type: "INSERT_SYNTHETIC", hunk, insertIndex });
  }, [dispatch]);

  useEffect(() => {
    onRegisterInsertSynthetic?.(handleInsertSynthetic);
  }, [handleInsertSynthetic, onRegisterInsertSynthetic]);

  // Create a modified diff with the expanded hunks
  const expandedDiff: ParsedDiff = {
    ...diff,
    hunks,
  };

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
