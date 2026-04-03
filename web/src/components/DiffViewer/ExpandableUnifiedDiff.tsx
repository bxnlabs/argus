import { memo, useEffect } from "react";
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
  onCommentRef?: (id: string, el: HTMLElement | null) => void;
  onExpandedHunksChange?: (hunks: DiffHunk[]) => void;
}

export const ExpandableUnifiedDiff = memo(function ExpandableUnifiedDiff({
  diff,
  totalLines: totalLinesProp,
  expansionContext,
  onExpandedHunksChange,
  ...unifiedDiffProps
}: ExpandableUnifiedDiffProps) {
  const { hunks, totalLines, expandLoading, expandErrors, handleExpand } = useExpandableDiff(
    diff.hunks,
    totalLinesProp,
    expansionContext,
  );

  // Report expanded hunks to parent for comment/snippet resolution
  useEffect(() => {
    onExpandedHunksChange?.(hunks);
  }, [hunks, onExpandedHunksChange]);

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
