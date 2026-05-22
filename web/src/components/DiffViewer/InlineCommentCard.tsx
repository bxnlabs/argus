import { useState } from "react";
import { AlertTriangle, Pencil, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import type { ReviewComment } from "@/types";
import { InlineCommentForm } from "./InlineCommentForm";

interface InlineCommentCardProps {
  comment: ReviewComment;
  onDelete?: (id: string) => void;
  onEdit?: (id: string, body: string) => void;
  /** When provided, clicking the edit button calls this instead of entering inline edit mode. */
  onEditRequest?: (comment: ReviewComment) => void;
}

export function InlineCommentCard({ comment, onDelete, onEdit, onEditRequest }: InlineCommentCardProps) {
  const isDraft = !comment.submitted;
  const isStale = comment.anchorStatus === "stale";
  const side = comment.line.from.side;
  const line = comment.line.from.line;
  const [isEditing, setIsEditing] = useState(false);

  return (
    <div
      className={cn(
        "border-l-4 px-3 py-2 font-sans",
        // State is carried by the left border alone — no card chrome:
        // foreground = submitted (the implied default), primary = pending
        // draft, yellow = stale anchor.
        isDraft
          ? "border-l-primary"
          : isStale
            ? "border-l-yellow-500"
            : "border-l-foreground",
      )}
    >
      {/* Header */}
      <div className="flex items-center gap-2 pb-1">
        <span className="text-muted-foreground text-xs">
          Comment on {side}{line}
        </span>
        {isDraft && (
          <span className="bg-primary/15 text-primary inline-flex items-center rounded-full px-1.5 py-0.5 text-[10px] font-medium tracking-wide uppercase">
            Pending
          </span>
        )}
        {isStale && (
          <span className="flex items-center gap-1 text-xs text-yellow-500">
            <AlertTriangle className="h-3 w-3" />
            Anchor may have moved
          </span>
        )}
        <span className="flex-1" />
        {onEdit && !isEditing && (
          <Button
            size="icon-sm"
            variant="ghost"
            onClick={() => onEditRequest ? onEditRequest(comment) : setIsEditing(true)}
            aria-label="Edit comment"
            className="text-muted-foreground hover:text-foreground -mr-1 h-6 w-6"
          >
            <Pencil className="h-3 w-3" />
          </Button>
        )}
        {onDelete && (
          <Button
            size="icon-sm"
            variant="ghost"
            onClick={() => onDelete(comment.id)}
            aria-label="Delete comment"
            className="text-muted-foreground hover:text-destructive -mr-1 h-6 w-6"
          >
            <X className="h-3 w-3" />
          </Button>
        )}
      </div>

      {/* Body */}
      {isEditing ? (
        <InlineCommentForm
          bare
          initialBody={comment.body}
          submitLabel="Save"
          onSubmit={(body) => {
            if (body !== comment.body) {
              onEdit?.(comment.id, body);
            }
            setIsEditing(false);
          }}
          onCancel={() => setIsEditing(false)}
        />
      ) : (
        <p className="text-foreground/90 whitespace-pre-wrap text-sm leading-relaxed">
          {comment.body}
        </p>
      )}
    </div>
  );
}
