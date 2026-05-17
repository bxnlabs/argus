import { memo } from "react";
import { type ParsedDiff, type DiffHunk } from "@/lib/diff-parser";
import type { ReviewComment, DiffPosition } from "@/types";
import type { ExpansionContext } from "@/hooks/useExpandableDiff";
import { ExpandableUnifiedDiff } from "./ExpandableUnifiedDiff";
import { useLazyMount } from "@/hooks/useLazyMount";
import { Plus, Minus, ChevronRight } from "lucide-react";
import { cn } from "@/lib/utils";

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
   * New-side line numbers to auto-expand context around on first mount.
   * Used to surface caseB comments inline (see ExpandableUnifiedDiff).
   */
  autoExpandLines?: number[];
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
          autoExpandLines={props.autoExpandLines}
        />
      ) : (
        <FilePlaceholder diff={props.diff} fileName={props.fileName} />
      )}
    </div>
  );
});

/** Lightweight placeholder matching the UnifiedDiff file header style. */
function FilePlaceholder({ diff, fileName }: { diff: ParsedDiff; fileName: string }) {
  return (
    <div
      className={cn(
        "border-border flex w-full items-center gap-2 border px-3 py-2.5 text-sm",
        "bg-muted text-left",
        "min-h-[44px]",
      )}
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
