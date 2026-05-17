import { memo, useMemo } from "react";
import { AlertTriangle } from "lucide-react";
import { InlineCommentCard } from "@/components/DiffViewer/InlineCommentCard";
import type { ReviewComment } from "@/types";

interface UnanchoredCommentSectionProps {
  /**
   * Comments that have no place inline — file is not in the current compare,
   * line is beyond EOF, or totalLines is missing. Caller is expected to pass
   * them already filtered (typically `partition.unanchored`).
   */
  comments: ReviewComment[];
  onDeleteComment?: (id: string) => void;
  onEditComment?: (id: string, body: string) => void;
  onEditCommentRequest?: (comment: ReviewComment) => void;
  onCommentRef?: (id: string, el: HTMLElement | null) => void;
}

/**
 * Renders comments that can't be anchored inline so the reviewer can still
 * read them and prune (delete) them. Groups by file path, preserving the
 * input order of comments (caller is responsible for ordering).
 *
 * Each card reuses the inline comment card to keep the edit/delete UX
 * consistent. The snippet (`snippetContext` if present, else `snippet`)
 * is shown above the card as a read-only code block so the reviewer
 * sees what the comment was originally pointing at.
 */
export const UnanchoredCommentSection = memo(function UnanchoredCommentSection({
  comments,
  onDeleteComment,
  onEditComment,
  onEditCommentRequest,
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
          <div key={filePath} className="space-y-1">
            <div className="text-muted-foreground px-1 font-mono text-xs">
              {filePath}
            </div>
            <ul className="space-y-2">
              {fileComments.map((c) => (
                <li
                  key={c.id}
                  ref={(el) => onCommentRef?.(c.id, el)}
                  className="border-border/60 bg-card/40 rounded-md border"
                >
                  <UnanchoredCommentHeader comment={c} />
                  <InlineCommentCard
                    comment={c}
                    onDelete={onDeleteComment}
                    onEdit={onEditComment}
                    onEditRequest={onEditCommentRequest}
                  />
                </li>
              ))}
            </ul>
          </div>
        ))}
      </div>
    </section>
  );
});

function UnanchoredCommentHeader({ comment }: { comment: ReviewComment }) {
  const side = comment.line.from.side;
  const line = comment.line.from.line;
  const snippetText = comment.snippetContext || comment.snippet;
  return (
    <div className="px-3 pt-2">
      <div className="text-muted-foreground flex items-center gap-2 text-xs">
        <span className="bg-muted rounded px-1.5 py-0.5 font-mono">
          {side}:{line}
        </span>
      </div>
      {snippetText && (
        <pre className="bg-muted/60 border-border/40 mt-1.5 overflow-x-auto rounded border px-2 py-1 font-mono text-[11px] leading-snug whitespace-pre">
          {snippetText}
        </pre>
      )}
    </div>
  );
}
