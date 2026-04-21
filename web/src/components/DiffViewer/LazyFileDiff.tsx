import { memo } from "react";
import { type ParsedDiff, type DiffHunk } from "@/lib/diff-parser";
import type { ReviewComment, DiffPosition } from "@/types";
import type { AutoExpandTarget } from "@/lib/compare-comments";
import type { ExpansionContext } from "@/hooks/useExpandableDiff";
import { ExpandableUnifiedDiff } from "./ExpandableUnifiedDiff";
import { useLazyMount } from "@/hooks/useLazyMount";
import { Plus, Minus, ChevronRight } from "lucide-react";
import { cn } from "@/lib/utils";

// Placeholder sizing — computed from diff.hunks (what UnifiedDiff actually
// renders), not from the source file's totalLines. This matches the plan's
// follow-up review: totalLines = full file line count (e.g. 2000 for a
// 2000-line file) would produce a hugely over-sized placeholder for a small
// change, and on iOS Safari without scroll anchoring a predecessor mounting
// above the viewport would shrink document height and jolt the user.
//
// Per-file overhead:
//   FILE_HEADER_HEIGHT_PX — sticky file header (min-h-[44px] in UnifiedDiff)
// Per-hunk overhead:
//   HUNK_OVERHEAD_PX     — hunk header row (~24px, py-1 + text-xs) plus
//                          amortized expand-row padding (~16px per hunk)
// Per-row overhead:
//   PLACEHOLDER_LINE_HEIGHT_PX — DiffLine row (text-xs 16px + py-0.5 4px)
//
// The sum is deliberately a close approximation, not exact. Slight overshoot
// (e.g. missing trailing expand-down row) is preferable to undershoot
// because scrollHeight stability only needs same-order-of-magnitude accuracy
// for the IntersectionObserver-driven lazy mount to behave well across
// browsers that do and don't support CSS scroll anchoring.
export const FILE_HEADER_HEIGHT_PX = 44;
export const PLACEHOLDER_LINE_HEIGHT_PX = 20;
export const HUNK_OVERHEAD_PX = 40;

interface LazyFileDiffProps {
  diff: ParsedDiff;
  fileName: string;
  wrapLines: boolean;
  comments: ReviewComment[];
  activeCommentLine: { position: DiffPosition } | null;
  onLineClick?: (position: DiffPosition) => void;
  onAddComment?: (body: string) => void;
  onCancelComment?: () => void;
  onDeleteComment?: (id: string) => void;
  onEditComment?: (id: string, body: string) => void;
  onEditCommentRequest?: (comment: ReviewComment) => void;
  onCommentRef?: (id: string, el: HTMLElement | null) => void;
  totalLines: number;
  onExpandedHunksChange?: (hunks: DiffHunk[]) => void;
  expansionContext: ExpansionContext;
  /** When true, skip lazy mounting and render content immediately. */
  forceMount?: boolean;
  /**
   * Coalesced context windows to auto-expand on first mount, surfacing caseB
   * comments inline (see ExpandableUnifiedDiff).
   */
  autoExpandTargets?: AutoExpandTarget[];
  /** Called with the full current set of comment IDs this file's auto-expansion can't surface inline. */
  onAutoExpandFailuresChange?: (commentIds: string[]) => void;
}

export const LazyFileDiff = memo(function LazyFileDiff(props: LazyFileDiffProps) {
  // 300px gives extra pre-load buffer so tall file diffs start mounting before scrolling into view
  const { ref, shouldMount } = useLazyMount("300px");
  const mounted = props.forceMount || shouldMount;

  return (
    <div ref={ref}>
      {mounted ? (
        <ExpandableUnifiedDiff
          diff={props.diff}
          fileName={props.fileName}
          expanded
          wrapLines={props.wrapLines}
          comments={props.comments}
          activeCommentLine={props.activeCommentLine}
          onLineClick={props.onLineClick}
          onAddComment={props.onAddComment}
          onCancelComment={props.onCancelComment}
          onDeleteComment={props.onDeleteComment}
          onEditComment={props.onEditComment}
          onEditCommentRequest={props.onEditCommentRequest}
          onCommentRef={props.onCommentRef}
          totalLines={props.totalLines}
          onExpandedHunksChange={props.onExpandedHunksChange}
          expansionContext={props.expansionContext}
          autoExpandTargets={props.autoExpandTargets}
          onAutoExpandFailuresChange={props.onAutoExpandFailuresChange}
        />
      ) : (
        <FilePlaceholder diff={props.diff} fileName={props.fileName} />
      )}
    </div>
  );
});

/**
 * Lightweight placeholder matching the UnifiedDiff file header style and
 * reserving vertical space equal to the mounted diff's approximate height.
 * The sizing lets predecessor and successor files stay lazy without layout
 * shift: scrollHeight is stable whether a file is a placeholder or mounted,
 * so scrollIntoView lands at the target's true offsetTop and
 * IntersectionObserver-driven mounts don't drift upward scroll on browsers
 * without scroll anchoring (iOS/Safari).
 */
export function FilePlaceholder({
  diff,
  fileName,
}: {
  diff: ParsedDiff;
  fileName: string;
}) {
  const rowCount = diff.hunks.reduce((n, h) => n + h.lines.length, 0);
  const hunkOverhead = diff.hunks.length * HUNK_OVERHEAD_PX;
  const minHeight =
    FILE_HEADER_HEIGHT_PX + rowCount * PLACEHOLDER_LINE_HEIGHT_PX + hunkOverhead;
  return (
    <div
      className={cn(
        "border-border flex w-full items-start gap-2 border px-3 py-2.5 text-sm",
        "bg-muted text-left",
      )}
      style={{ minHeight: `${minHeight}px` }}
    >
      <ChevronRight className="text-muted-foreground h-4 w-4 flex-shrink-0" />
      <span className="flex-1 truncate text-xs font-medium">{fileName}</span>
      <span className="flex flex-shrink-0 items-center gap-2 text-xs">
        {diff.additions > 0 && (
          <span className="flex items-center gap-0.5 text-green-500">
            <Plus className="h-3 w-3" />
            {diff.additions}
          </span>
        )}
        {diff.deletions > 0 && (
          <span className="flex items-center gap-0.5 text-red-500">
            <Minus className="h-3 w-3" />
            {diff.deletions}
          </span>
        )}
      </span>
    </div>
  );
}
