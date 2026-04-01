import { useState, useEffect, Fragment, memo, useMemo } from "react";
import { ChevronDown, ChevronUp, ChevronRight, Plus, Minus, Loader2 } from "lucide-react";
import { cn } from "@/lib/utils";
import type { ParsedDiff, DiffHunk, DiffLine } from "@/lib/diff-parser";
import type { ReviewComment } from "@/types";
import type { ExpandDirection } from "@/hooks/useExpandableDiff";
import { InlineCommentForm } from "./InlineCommentForm";
import { InlineCommentCard } from "./InlineCommentCard";

const EMPTY_COMMENTS: ReviewComment[] = [];

interface UnifiedDiffProps {
  diff: ParsedDiff;
  fileName: string;
  expanded?: boolean;
  onToggle?: () => void;
  wrapLines?: boolean;
  comments?: ReviewComment[];
  activeCommentLine?: { from: number; to: number } | null;
  onLineClick?: (line: number) => void;
  onAddComment?: (body: string) => void;
  onCancelComment?: () => void;
  onDeleteComment?: (id: string) => void;
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
    const map = new Map<number, ReviewComment[]>();
    for (const c of effectiveComments) {
      const line = c.line.to;
      const arr = map.get(line);
      if (arr) arr.push(c);
      else map.set(line, [c]);
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
                  <Fragment key={index}>
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

function Hunk({
  hunk,
  wrapLines,
  commentsByLine,
  activeCommentLine,
  onLineClick,
  onAddComment,
  onCancelComment,
  onDeleteComment,
  onCommentRef,
  commentingEnabled,
}: {
  hunk: DiffHunk;
  wrapLines: boolean;
  commentsByLine: Map<number, ReviewComment[]> | null;
  activeCommentLine: { from: number; to: number } | null;
  onLineClick?: (line: number) => void;
  onAddComment?: (body: string) => void;
  onCancelComment?: () => void;
  onDeleteComment?: (id: string) => void;
  onCommentRef?: (id: string, el: HTMLElement | null) => void;
  commentingEnabled: boolean;
}) {
  return (
    <div className="min-w-full">
      <div className="border-border border-y bg-blue-500/10 px-3 py-1 text-xs text-blue-400">
        {hunk.header}
      </div>
      {hunk.lines.map((line, index) => {
        const newLine = line.newLineNumber;
        const isInActiveRange =
          activeCommentLine != null &&
          newLine != null &&
          newLine >= activeCommentLine.from &&
          newLine <= activeCommentLine.to;

        const lineComments =
          newLine != null && commentsByLine
            ? commentsByLine.get(newLine) ?? EMPTY_COMMENTS
            : EMPTY_COMMENTS;

        const showForm =
          activeCommentLine != null && newLine === activeCommentLine.to;

        return (
          <Fragment key={index}>
            <DiffLineRow
              line={line}
              wrapLines={wrapLines}
              isInActiveRange={isInActiveRange}
              onLineClick={onLineClick}
              commentingEnabled={commentingEnabled}
            />
            {lineComments.map((c) => (
              <div
                key={c.id}
                ref={(el) => onCommentRef?.(c.id, el)}
                className={cn(!wrapLines && "sticky left-0")}
                style={!wrapLines ? { width: "calc(100vw - 0.75rem * 2 - 2px)" } : undefined}
              >
                <InlineCommentCard comment={c} onDelete={onDeleteComment!} />
              </div>
            ))}
            {showForm && onAddComment && onCancelComment && (
              <div className={cn(!wrapLines && "sticky left-0")}
                style={!wrapLines ? { width: "calc(100vw - 0.75rem * 2 - 2px)" } : undefined}>
                <InlineCommentForm
                  onSubmit={onAddComment}
                  onCancel={onCancelComment}
                />
              </div>
            )}
          </Fragment>
        );
      })}
    </div>
  );
}

function DiffLineRow({
  line,
  wrapLines,
  isInActiveRange,
  onLineClick,
  commentingEnabled,
}: {
  line: DiffLine;
  wrapLines: boolean;
  isInActiveRange: boolean;
  onLineClick?: (line: number) => void;
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
    line.type !== "deletion" &&
    line.newLineNumber != null &&
    line.content.trim() !== "";

  return (
    <div className={cn("flex hover:bg-muted/30", bgColor, isInActiveRange && "bg-blue-500/10")}>
      <div className="text-muted-foreground border-border/50 w-10 shrink-0 border-r px-2 py-0.5 text-right tabular-nums select-none">
        {line.oldLineNumber ?? ""}
      </div>
      <div
        className={cn(
          "text-muted-foreground border-border/50 w-10 shrink-0 border-r px-2 py-0.5 text-right tabular-nums select-none",
          isCommentable && "cursor-pointer hover:bg-blue-500/20 hover:text-blue-400",
        )}
        onClick={
          isCommentable
            ? () => onLineClick?.(line.newLineNumber!)
            : undefined
        }
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
}

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
