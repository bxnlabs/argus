import { useState, useEffect, Fragment } from "react";
import { ChevronDown, ChevronRight, Plus, Minus } from "lucide-react";
import { cn } from "@/lib/utils";
import type { ParsedDiff, DiffHunk, DiffLine } from "@/lib/diff-parser";
import type { ReviewComment } from "@/types";
import { InlineCommentForm } from "./InlineCommentForm";
import { InlineCommentCard } from "./InlineCommentCard";

interface UnifiedDiffProps {
  diff: ParsedDiff;
  fileName: string;
  expanded?: boolean;
  onToggle?: () => void;
  wrapLines?: boolean;
  // Comment props (optional — when absent, commenting is disabled)
  comments?: ReviewComment[];
  activeCommentLine?: { from: number; to: number } | null;
  onLineClick?: (line: number, shiftKey: boolean) => void;
  onAddComment?: (body: string) => void;
  onCancelComment?: () => void;
  onDeleteComment?: (id: string) => void;
  onCommentRef?: (id: string, el: HTMLElement | null) => void;
}

export function UnifiedDiff({
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

  return (
    <div className="border-border overflow-hidden rounded-lg border">
      {/* File header */}
      <button
        onClick={handleToggle}
        className={cn(
          "flex w-full items-center gap-2 px-3 py-2.5 text-sm",
          "bg-muted/50 hover:bg-muted text-left transition-colors",
          "min-h-[44px]",
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
        <div className={wrapLines ? "overflow-hidden" : "overflow-x-auto"}>
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
              {diff.hunks.map((hunk, index) => (
                <Hunk
                  key={index}
                  hunk={hunk}
                  wrapLines={wrapLines}
                  comments={comments ?? []}
                  activeCommentLine={activeCommentLine ?? null}
                  onLineClick={onLineClick}
                  onAddComment={onAddComment}
                  onCancelComment={onCancelComment}
                  onDeleteComment={onDeleteComment}
                  onCommentRef={onCommentRef}
                  commentingEnabled={commentingEnabled}
                />
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
}

function Hunk({
  hunk,
  wrapLines,
  comments,
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
  comments: ReviewComment[];
  activeCommentLine: { from: number; to: number } | null;
  onLineClick?: (line: number, shiftKey: boolean) => void;
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
          newLine != null
            ? comments.filter((c) => c.line.to === newLine)
            : [];

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
  onLineClick?: (line: number, shiftKey: boolean) => void;
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
    line.newLineNumber != null;

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
            ? (e) => onLineClick?.(line.newLineNumber!, e.shiftKey)
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
