import { memo, useMemo } from "react";
import { AlertTriangle } from "lucide-react";
import { InlineCommentCard } from "@/components/DiffViewer/InlineCommentCard";
import { cn } from "@/lib/utils";
import type { ReviewComment } from "@/types";

interface UnanchoredCommentSectionProps {
  /**
   * Comments that have no place inline — file is not in the current compare,
   * line is beyond EOF, or totalLines is missing. Caller is expected to pass
   * them already filtered (typically `partition.unanchored`).
   */
  comments: ReviewComment[];
  onDeleteComment?: (id: string) => void;
  onCommentRef?: (id: string, el: HTMLElement | null) => void;
}

/**
 * Renders comments that can't be anchored inline so the reviewer can still
 * read them and prune (delete) them. Each file group mirrors the compare's
 * diff chrome — a file header, the stored snippet as context-style lines, then
 * the comment card — so it reads consistently with anchored comments.
 *
 * It is deliberately monochrome with no +/- markers: an unanchored comment has
 * no add/delete state in the current diff, and its line number is the stored
 * anchor, not a live position. Editing is not offered (read + prune only).
 */
export const UnanchoredCommentSection = memo(function UnanchoredCommentSection({
  comments,
  onDeleteComment,
  onCommentRef,
}: UnanchoredCommentSectionProps) {
  // Group by file path in the order comments appear, so the caller controls
  // the visual order via the sorted `comments` array.
  const groups = useMemo(() => {
    const map = new Map<string, ReviewComment[]>();
    for (const c of comments) {
      // For L-side comments, prefer oldPath; for R-side, use file.
      const key = c.line.from.side === "L" ? c.oldPath ?? c.file : c.file;
      const arr = map.get(key);
      if (arr) arr.push(c);
      else map.set(key, [c]);
    }
    return Array.from(map.entries());
  }, [comments]);

  if (comments.length === 0) return null;

  return (
    <section className="mt-6">
      <div className="flex items-center gap-2 border-b border-yellow-500/30 px-1 pb-1.5 text-sm">
        <AlertTriangle className="h-4 w-4 text-yellow-500" />
        <h3 className="font-medium">
          Unanchored comments{" "}
          <span className="text-muted-foreground">({comments.length})</span>
        </h3>
      </div>
      <p className="text-muted-foreground px-1 pt-1.5 text-xs">
        These comments were authored against code that's no longer present in
        the current compare. Read them and delete what no longer applies.
      </p>
      <div className="mt-3 space-y-4">
        {groups.map(([filePath, fileComments]) => (
          <div
            key={filePath}
            className="border-border overflow-hidden rounded-lg border"
          >
            <div className="border-border bg-muted text-muted-foreground border-b px-3 py-2 font-mono text-xs font-medium">
              {filePath}
            </div>
            {fileComments.map((c, i) => (
              <div
                key={c.id}
                ref={(el) => onCommentRef?.(c.id, el)}
                className={cn(i > 0 && "border-border/50 border-t")}
              >
                <UnanchoredSnippet comment={c} />
                <InlineCommentCard comment={c} onDelete={onDeleteComment} />
              </div>
            ))}
          </div>
        ))}
      </div>
    </section>
  );
});

/**
 * Renders the comment's stored snippet in the style of diff context lines: the
 * two-column old│new gutter plus monospace content. The anchor's side decides
 * which gutter column holds the line number (L on the left, R on the right),
 * mirroring how the diff places deletions and additions. Monochrome with no
 * +/- markers — an unanchored comment has no add/delete state — and the anchor
 * line gets a subtle highlight.
 */
function UnanchoredSnippet({ comment }: { comment: ReviewComment }) {
  const side = comment.line.from.side;
  const anchorLine = comment.line.from.line;
  const text = comment.snippetContext || comment.snippet;
  if (!text) return null;

  const lines = text.split("\n");
  // `snippetContext` is captured as a consecutive run around the anchor and
  // `snippet` is the anchor line itself, so locating it lets us number the run.
  const anchorIdx = comment.snippet ? lines.indexOf(comment.snippet) : -1;

  return (
    <div className="overflow-x-auto font-mono text-xs">
      {lines.map((content, i) => {
        const isAnchor = anchorIdx >= 0 ? i === anchorIdx : i === 0;
        const lineNo =
          anchorIdx >= 0 ? anchorLine + (i - anchorIdx) : isAnchor ? anchorLine : null;
        const num = lineNo ?? "";
        return (
          <div key={i} className={cn("flex", isAnchor && "bg-yellow-500/10")}>
            <div className="text-muted-foreground border-border/50 w-10 shrink-0 border-r px-2 py-0.5 text-right tabular-nums select-none">
              {side === "L" ? num : ""}
            </div>
            <div className="text-muted-foreground border-border/50 w-10 shrink-0 border-r px-2 py-0.5 text-right tabular-nums select-none">
              {side === "R" ? num : ""}
            </div>
            <div className="text-foreground min-w-0 flex-1 px-2 py-0.5 whitespace-pre">
              {content || " "}
            </div>
          </div>
        );
      })}
    </div>
  );
}
