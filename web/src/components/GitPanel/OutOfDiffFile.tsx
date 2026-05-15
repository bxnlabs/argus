import { memo } from "react";
import { FileWarning } from "lucide-react";
import { InlineCommentCard } from "@/components/DiffViewer/InlineCommentCard";
import { cn } from "@/lib/utils";
import type { ReviewComment } from "@/types";

interface Props {
  groupKey: string;
  displayFile: string;
  comments: ReviewComment[];
  onDeleteComment: (id: string) => void;
  onEditComment: (id: string, body: string) => void;
  onEditCommentRequest?: (comment: ReviewComment) => void;
  onCommentRef?: (id: string, el: HTMLElement | null) => void;
}

// Renders comments whose anchor is not hosted by any rendered diff hunk — either
// because the file isn't in the compare diff (case a) or because the in-diff
// synthetic-hunk fetch for the anchor failed (case b). The stored snippet is
// the line content captured at comment authoring time; we render it as a
// diff-styled row so the orphan card lines up visually with real diff hunks.
export const OutOfDiffFile = memo(function OutOfDiffFile(props: Props) {
  return (
    <div data-out-of-diff-key={props.groupKey} className="border-border mb-3 rounded border">
      <div className="bg-muted text-muted-foreground flex items-center gap-2 border-b px-3 py-2 text-xs">
        <FileWarning className="h-3.5 w-3.5" />
        <span className="font-medium">{props.displayFile}</span>
        <span className="italic">not in compare diff — showing stored snippet</span>
      </div>
      <div className="space-y-2 py-2">
        {props.comments.map((c) => (
          <div key={c.id} ref={(el) => props.onCommentRef?.(c.id, el)}>
            <SnippetDiffRow comment={c} />
            <div className="mx-3 mt-2">
              <InlineCommentCard
                comment={c}
                onDelete={props.onDeleteComment}
                onEdit={props.onEditComment}
                onEditRequest={props.onEditCommentRequest}
              />
            </div>
          </div>
        ))}
      </div>
    </div>
  );
});

// Renders the stored snippet with the same row structure as a real diff line
// (two line-number gutters, marker column, content column) so an orphan card
// aligns with rendered hunks elsewhere on the page. The line-number gutter
// shows the comment's anchor line on the side it was authored against — that
// line may no longer exist at the current ref, but it's the snapshot the
// comment was made against, which is the same contract real diff hunks honor.
function SnippetDiffRow({ comment }: { comment: ReviewComment }) {
  const side = comment.line.from.side;
  const isAdd = side === "R";
  const isDel = side === "L";
  const marker = isAdd ? "+" : isDel ? "-" : " ";
  const bgColor = isAdd ? "bg-green-500/10" : isDel ? "bg-red-500/10" : "";
  const textColor = isAdd ? "text-green-400" : isDel ? "text-red-400" : "text-foreground";
  // Single-line is the common case (API enforces single-line comments). Split
  // defensively so any legacy multi-line snippet still renders cleanly.
  const snippetLines = comment.snippet.split("\n");
  const startLine = comment.line.from.line;
  return (
    <div className="font-mono text-xs">
      {snippetLines.map((content, i) => {
        const lineNum = startLine + i;
        return (
          <div key={i} className={cn("flex", bgColor)}>
            <div className="text-muted-foreground border-border/50 w-10 shrink-0 border-r px-2 py-0.5 text-right tabular-nums select-none">
              {isDel ? lineNum : ""}
            </div>
            <div className="text-muted-foreground border-border/50 w-10 shrink-0 border-r px-2 py-0.5 text-right tabular-nums select-none">
              {isAdd ? lineNum : ""}
            </div>
            <div className={cn("w-5 shrink-0 px-1 py-0.5 text-center select-none", textColor)}>
              {marker}
            </div>
            <div className={cn("min-w-0 flex-1 px-2 py-0.5 whitespace-pre", textColor)}>
              {content || " "}
            </div>
          </div>
        );
      })}
    </div>
  );
}
