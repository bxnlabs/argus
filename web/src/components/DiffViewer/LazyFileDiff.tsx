import { memo } from "react";
import { type ParsedDiff, type DiffHunk } from "@/lib/diff-parser";
import type { ReviewComment, DiffPosition } from "@/types";
import type { AutoExpandTarget } from "@/lib/compare-comments";
import type { ExpansionContext } from "@/hooks/useExpandableDiff";
import { ExpandableUnifiedDiff } from "./ExpandableUnifiedDiff";
import { useLazyMount } from "@/hooks/useLazyMount";
import { Plus, Minus, ChevronRight } from "lucide-react";
import { cn } from "@/lib/utils";

// Placeholder sizing. A lazy file is a short placeholder until it scrolls near
// the viewport; we reserve roughly the height its mounted diff will occupy so
// scrollHeight stays stable. The estimate is NOT relied on for pixel-accurate
// scroll positioning — CompareView re-aligns the scroll target against the pane
// after mounts settle (see its scroll-correction effect). Its only jobs here
// are to keep scrollHeight large enough that a near-bottom target isn't clamped,
// and to avoid gross layout jumps. Sized from diff.hunks (what UnifiedDiff
// renders), never the source file's totalLines.
const FILE_HEADER_HEIGHT_PX = 44; // sticky file header (min-h-[44px])
const ROW_LINE_HEIGHT_PX = 16; // text-xs line-height for one visual line
const ROW_PADDING_PX = 4; // py-0.5 applied once per logical DiffLine row
const HUNK_HEADER_HEIGHT_PX = 26; // @@ header row: text-xs 16 + py-1 8 + border-y 2
const EXPAND_ROW_HEIGHT_PX = 20; // expand affordance: h-3.5 icon 14 + py-0.5 4 + border-y 2
const EMPTY_BODY_HEIGHT_PX = 85; // binary / "No changes": px-4 py-8 + text 84 + wrapper border-b 1
const BODY_WRAPPER_BORDER_PX = 1; // diff body wrapper border-b
// Assumed monospace columns in the diff content area, used only to approximate
// how many visual rows a long line wraps into when wrapLines is on. A coarse
// overshoot-biased clamp guard, NOT a correctness mechanism (the scroll
// correction in CompareView absorbs the residual). Caveat: counts UTF-16 code
// units, so tabs and wide/CJK glyphs are under-counted, and the true column
// count depends on the resizable panel width.
const WRAP_COLS = 80;

/** Visual rows a logical diff line occupies, given the wrap mode. */
function visualRows(content: string, wrapLines: boolean): number {
  if (!wrapLines) return 1;
  return Math.max(1, Math.ceil(content.length / WRAP_COLS));
}

/**
 * Initial expand-context rows UnifiedDiff renders for a freshly-mounted diff.
 * Mirrors the showExpandUp / gap / showExpandDown conditions in
 * UnifiedDiff.tsx (see its hunk map) — keep the two in sync. At placeholder
 * time there are no expand errors and onExpand is always wired, so the count is
 * deterministic from hunk geometry and totalLines.
 */
function initialExpandRowCount(diff: ParsedDiff, totalLines: number): number {
  const hunks = diff.hunks;
  let rows = 0;
  if (hunks[0].newStart > 1) rows += 1; // expand-up above the first hunk
  for (let i = 1; i < hunks.length; i++) {
    const prev = hunks[i - 1];
    if (prev.newStart + prev.newCount < hunks[i].newStart) rows += 2; // gap: down + up
  }
  const last = hunks[hunks.length - 1];
  if (totalLines > 0 && last.newCount > 0 && last.newStart + last.newCount - 1 < totalLines) {
    rows += 1; // expand-down below the last hunk
  }
  return rows;
}

/**
 * Approximate the mounted height of a file diff, for placeholder sizing.
 * `wrapLines` mirrors UnifiedDiff: when on, long lines wrap to multiple visual
 * rows; when off they scroll horizontally and occupy exactly one row.
 */
export function estimatePlaceholderMinHeight(
  diff: ParsedDiff,
  wrapLines: boolean,
  totalLines: number,
): number {
  // Binary and zero-hunk diffs render a centered message body, not rows.
  if (diff.isBinary || diff.hunks.length === 0) {
    return FILE_HEADER_HEIGHT_PX + EMPTY_BODY_HEIGHT_PX;
  }

  let bodyHeight = 0;
  for (const hunk of diff.hunks) {
    for (const line of hunk.lines) {
      // header-type lines render null in UnifiedDiff (the @@ heading shows in
      // the hunk header row instead), so they take no body row.
      if (line.type === "header") continue;
      // py-0.5 padding applies once per logical row; each wrapped visual line
      // adds one line-height. One row => 20px; each extra wrapped line => +16px.
      bodyHeight += ROW_LINE_HEIGHT_PX * visualRows(line.content, wrapLines) + ROW_PADDING_PX;
    }
  }

  const hunkHeaders = diff.hunks.length * HUNK_HEADER_HEIGHT_PX;
  const expandRows = initialExpandRowCount(diff, totalLines) * EXPAND_ROW_HEIGHT_PX;

  return FILE_HEADER_HEIGHT_PX + hunkHeaders + expandRows + bodyHeight + BODY_WRAPPER_BORDER_PX;
}

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
        <FilePlaceholder
          diff={props.diff}
          fileName={props.fileName}
          wrapLines={props.wrapLines}
          totalLines={props.totalLines}
        />
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
  wrapLines,
  totalLines,
}: {
  diff: ParsedDiff;
  fileName: string;
  wrapLines: boolean;
  totalLines: number;
}) {
  const minHeight = estimatePlaceholderMinHeight(diff, wrapLines, totalLines);
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
