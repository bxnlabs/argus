import { useState, useEffect, Fragment, memo, useMemo, useCallback } from "react";
import { ChevronDown, ChevronUp, ChevronRight, Plus, Minus, Loader2 } from "lucide-react";
import { cn } from "@/lib/utils";
import type { ParsedDiff, DiffHunk, DiffLine } from "@/lib/diff-parser";
import type { ReviewComment, DiffPosition } from "@/types";
import type { ExpandDirection } from "@/hooks/useExpandableDiff";
import { InlineCommentForm } from "./InlineCommentForm";
import { InlineCommentCard } from "./InlineCommentCard";
import { InlineCommentFrame } from "./InlineCommentFrame";

const EMPTY_COMMENTS: ReviewComment[] = [];

interface UnifiedDiffProps {
  diff: ParsedDiff;
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
  onExpand?: (direction: ExpandDirection, hunkIndex: number) => void;
  expandLoading?: Record<string, boolean>;
  expandErrors?: Record<string, "permanent" | "transient">;
  totalLines?: number;
}

export const UnifiedDiff = memo(function UnifiedDiff({
  diff,
  fileName,
  expanded = true,
  onToggle,
  wrapLines = true,
  comments,
  activeCommentLine,
  onLineClick,
  onAddComment,
  onCancelComment,
  onDeleteComment,
  onEditComment,
  onEditCommentRequest,
  onCommentRef,
  onExpand,
  expandLoading,
  expandErrors,
  totalLines,
}: UnifiedDiffProps) {
  const [localExpanded, setLocalExpanded] = useState(expanded);
  useEffect(() => {
    if (!onToggle) setLocalExpanded(expanded);
  }, [expanded, onToggle]);
  const isExpanded = onToggle ? expanded : localExpanded;

  const handleToggle = () => {
    if (onToggle) {
      onToggle();
    } else {
      setLocalExpanded(!localExpanded);
    }
  };

  const commentingEnabled = !!onLineClick;

  // Pre-index comments by line number — O(comments) once instead of O(lines * comments)
  const effectiveComments = comments ?? EMPTY_COMMENTS;
  const commentsByLine = useMemo(() => {
    if (effectiveComments.length === 0) return null;
    const map = new Map<string, ReviewComment[]>();
    for (const c of effectiveComments) {
      const key = `${c.line.to.side}${c.line.to.line}`;
      const arr = map.get(key);
      if (arr) arr.push(c);
      else map.set(key, [c]);
    }
    return map;
  }, [effectiveComments]);

  return (
    <div>
      {/* File header */}
      <button
        onClick={handleToggle}
        className={cn(
          "border-border flex w-full items-center gap-2 border px-3 py-2.5 text-sm",
          "bg-muted hover:bg-muted/80 text-left transition-colors",
          "sticky top-0 z-20 min-h-[44px]",
        )}
      >
        {isExpanded ? (
          <ChevronDown className="text-muted-foreground h-4 w-4 flex-shrink-0" />
        ) : (
          <ChevronRight className="text-muted-foreground h-4 w-4 flex-shrink-0" />
        )}

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
      </button>

      {isExpanded && (
        <div className={cn("border-border rounded-b-lg border-x border-b", wrapLines ? "overflow-hidden" : "overflow-x-auto")}>
          {diff.isBinary ? (
            <div className="text-muted-foreground px-4 py-8 text-center text-sm">
              Binary file not shown
            </div>
          ) : diff.hunks.length === 0 ? (
            <div className="text-muted-foreground px-4 py-8 text-center text-sm">
              No changes
            </div>
          ) : (
            <div className={cn("min-w-full font-mono text-xs", !wrapLines && "w-fit")}>
              {diff.hunks.map((hunk, index) => {
                const upKey = `up-${index}`;
                const downKey = `down-${index}`;
                const prevDownKey = `down-${index - 1}`;

                const showExpandUp = onExpand && index === 0 && hunk.newStart > 1 &&
                  expandErrors?.[upKey] !== "permanent";

                // Between hunks: show expand-down for prev hunk and expand-up for this hunk
                const hasGap = onExpand && index > 0 && (() => {
                  const prev = diff.hunks[index - 1];
                  return prev.newStart + prev.newCount < hunk.newStart;
                })();
                const showGapDown = hasGap && expandErrors?.[prevDownKey] !== "permanent";
                const showGapUp = hasGap && expandErrors?.[upKey] !== "permanent";

                const showExpandDown = onExpand && index === diff.hunks.length - 1 &&
                  totalLines != null && totalLines > 0 &&
                  hunk.newCount > 0 &&
                  hunk.newStart + hunk.newCount - 1 < totalLines &&
                  expandErrors?.[downKey] !== "permanent";

                return (
                  <Fragment key={hunk.stableKey}>
                    {showExpandUp && (
                      <ExpandRow
                        direction="up"
                        hunkIndex={0}
                        loading={expandLoading?.[upKey] ?? false}
                        error={expandErrors?.[upKey]}
                        onExpand={onExpand!}
                      />
                    )}
                    {showGapDown && (
                      <ExpandRow
                        direction="down"
                        hunkIndex={index - 1}
                        loading={expandLoading?.[prevDownKey] ?? false}
                        error={expandErrors?.[prevDownKey]}
                        onExpand={onExpand!}
                      />
                    )}
                    {showGapUp && (
                      <ExpandRow
                        direction="up"
                        hunkIndex={index}
                        loading={expandLoading?.[upKey] ?? false}
                        error={expandErrors?.[upKey]}
                        onExpand={onExpand!}
                      />
                    )}
                    <Hunk
                      hunk={hunk}
                      wrapLines={wrapLines}
                      commentsByLine={commentsByLine}
                      activeCommentLine={activeCommentLine ?? null}
                      onLineClick={onLineClick}
                      onAddComment={onAddComment}
                      onCancelComment={onCancelComment}
                      onDeleteComment={onDeleteComment}
                      onEditComment={onEditComment}
                      onEditCommentRequest={onEditCommentRequest}
                      onCommentRef={onCommentRef}
                      commentingEnabled={commentingEnabled}
                    />
                    {showExpandDown && (
                      <ExpandRow
                        direction="down"
                        hunkIndex={index}
                        loading={expandLoading?.[downKey] ?? false}
                        error={expandErrors?.[downKey]}
                        onExpand={onExpand!}
                      />
                    )}
                  </Fragment>
                );
              })}
            </div>
          )}
        </div>
      )}
    </div>
  );
});

const Hunk = memo(function Hunk({
  hunk,
  wrapLines,
  commentsByLine,
  activeCommentLine,
  onLineClick,
  onAddComment,
  onCancelComment,
  onDeleteComment,
  onEditComment,
  onEditCommentRequest,
  onCommentRef,
  commentingEnabled,
}: {
  hunk: DiffHunk;
  wrapLines: boolean;
  commentsByLine: Map<string, ReviewComment[]> | null;
  activeCommentLine: { position: DiffPosition } | null;
  onLineClick?: (position: DiffPosition) => void;
  onAddComment?: (body: string) => void;
  onCancelComment?: () => void;
  onDeleteComment?: (id: string) => void;
  onEditComment?: (id: string, body: string) => void;
  onEditCommentRequest?: (comment: ReviewComment) => void;
  onCommentRef?: (id: string, el: HTMLElement | null) => void;
  commentingEnabled: boolean;
}) {
  const handleClick = useCallback(
    (e: React.MouseEvent<HTMLDivElement>) => {
      const target = e.target as HTMLElement;
      const el = target.closest<HTMLElement>("[data-comment-side]");
      if (!el) return;
      const side = el.dataset.commentSide as "L" | "R";
      const line = Number(el.dataset.commentLine);
      if (side && !isNaN(line)) {
        onLineClick?.({ side, line });
      }
    },
    [onLineClick],
  );

  return (
    <div className="min-w-full" onClick={commentingEnabled ? handleClick : undefined}>
      <div className="border-border border-y bg-blue-500/10 px-3 py-1 text-xs text-blue-400">
        {hunk.header}
      </div>
      {hunk.lines.map((line, index) => {
        const activePos = activeCommentLine?.position ?? null;
        const isInActiveRange =
          activePos != null &&
          ((activePos.side === "L" && line.oldLineNumber === activePos.line) ||
           (activePos.side === "R" && line.newLineNumber === activePos.line));

        const lComments =
          line.oldLineNumber != null && commentsByLine
            ? commentsByLine.get(`L${line.oldLineNumber}`)
            : undefined;
        const rComments =
          line.newLineNumber != null && commentsByLine
            ? commentsByLine.get(`R${line.newLineNumber}`)
            : undefined;
        const lineComments =
          lComments || rComments
            ? [...(lComments ?? []), ...(rComments ?? [])]
            : EMPTY_COMMENTS;

        const showForm = isInActiveRange;

        return (
          <Fragment key={`${line.type}:${line.oldLineNumber ?? ""}:${line.newLineNumber ?? ""}:${index}`}>
            <DiffLineRow
              line={line}
              wrapLines={wrapLines}
              isInActiveRange={isInActiveRange}
              commentingEnabled={commentingEnabled}
            />
            {lineComments.map((c) => (
              <div
                key={c.id}
                ref={(el) => onCommentRef?.(c.id, el)}
                className={cn(!wrapLines && "sticky left-0")}
                style={!wrapLines ? { width: "calc(100vw - 0.75rem * 2 - 2px)" } : undefined}
              >
                <InlineCommentCard comment={c} onDelete={onDeleteComment} onEdit={onEditComment} onEditRequest={onEditCommentRequest} />
              </div>
            ))}
            {showForm && activePos && onAddComment && onCancelComment && (
              <div className={cn(!wrapLines && "sticky left-0")}
                style={!wrapLines ? { width: "calc(100vw - 0.75rem * 2 - 2px)" } : undefined}>
                <InlineCommentFrame side={activePos.side} line={activePos.line} isDraft>
                  <InlineCommentForm
                    onSubmit={onAddComment}
                    onCancel={onCancelComment}
                  />
                </InlineCommentFrame>
              </div>
            )}
          </Fragment>
        );
      })}
    </div>
  );
});

const DiffLineRow = memo(function DiffLineRow({
  line,
  wrapLines,
  isInActiveRange,
  commentingEnabled,
}: {
  line: DiffLine;
  wrapLines: boolean;
  isInActiveRange: boolean;
  commentingEnabled: boolean;
}) {
  if (line.type === "header") return null;

  const bgColor =
    line.type === "addition"
      ? "bg-green-500/10"
      : line.type === "deletion"
        ? "bg-red-500/10"
        : "";

  const textColor =
    line.type === "addition"
      ? "text-green-400"
      : line.type === "deletion"
        ? "text-red-400"
        : "text-foreground";

  const marker =
    line.type === "addition" ? "+" : line.type === "deletion" ? "-" : "";

  const isCommentable =
    commentingEnabled &&
    (line.oldLineNumber != null || line.newLineNumber != null) &&
    line.content.trim() !== "";

  const isDeletion = line.type === "deletion";

  return (
    <div className={cn("flex hover:bg-muted/30", bgColor, isInActiveRange && "bg-blue-500/10")}>
      <div
        className={cn(
          "text-muted-foreground border-border/50 w-10 shrink-0 border-r px-2 py-0.5 text-right tabular-nums select-none",
          isCommentable && isDeletion && "cursor-pointer hover:bg-blue-500/20 hover:text-blue-400",
        )}
        data-comment-side={isCommentable && isDeletion ? "L" : undefined}
        data-comment-line={isCommentable && isDeletion ? line.oldLineNumber : undefined}
      >
        {isDeletion ? (line.oldLineNumber ?? "") : ""}
      </div>
      <div
        className={cn(
          "text-muted-foreground border-border/50 w-10 shrink-0 border-r px-2 py-0.5 text-right tabular-nums select-none",
          isCommentable && !isDeletion && "cursor-pointer hover:bg-blue-500/20 hover:text-blue-400",
        )}
        data-comment-side={isCommentable && !isDeletion ? "R" : undefined}
        data-comment-line={isCommentable && !isDeletion ? line.newLineNumber : undefined}
      >
        {line.newLineNumber ?? ""}
      </div>
      <div className={cn("w-5 shrink-0 px-1 py-0.5 text-center select-none", textColor)}>
        {marker}
      </div>
      <div className={cn(
        "min-w-0 flex-1 px-2 py-0.5",
        wrapLines ? "whitespace-pre-wrap break-words" : "whitespace-pre",
        textColor,
      )}>
        {line.content || " "}
      </div>
    </div>
  );
});

function ExpandRow({
  direction,
  hunkIndex,
  loading,
  error,
  onExpand,
}: {
  direction: ExpandDirection;
  hunkIndex: number;
  loading: boolean;
  error?: "permanent" | "transient";
  onExpand: (direction: ExpandDirection, hunkIndex: number) => void;
}) {
  const DirectionIcon = direction === "up" ? ChevronUp : ChevronDown;
  const ariaLabel = error === "transient"
    ? "Retry — click to try again"
    : `Expand ${direction}`;

  return (
    <div className={cn(
      "border-border/50 flex items-center border-y",
      error === "transient" && "bg-red-500/10",
    )}>
      <button
        onClick={() => onExpand(direction, hunkIndex)}
        disabled={loading}
        aria-label={ariaLabel}
        className={cn(
          "border-border/50 flex w-20 shrink-0 items-center justify-center border-r py-0.5",
          "text-blue-400 bg-blue-500/10 hover:bg-blue-500/20 hover:text-blue-300",
          "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset",
          "disabled:opacity-50 disabled:cursor-not-allowed",
          "transition-colors cursor-pointer",
          error === "transient" && "bg-red-500/10 text-red-400 hover:bg-red-500/20 hover:text-red-300",
        )}
      >
        {loading ? (
          <Loader2 className="h-3.5 w-3.5 animate-spin" />
        ) : (
          <DirectionIcon className="h-3.5 w-3.5" />
        )}
      </button>
      {error === "transient" && (
        <>
          <div className="w-5 shrink-0" />
          <div className="flex-1 px-2 py-0.5 text-xs text-red-400 select-none">
            Failed to load — click to retry
          </div>
        </>
      )}
    </div>
  );
}
