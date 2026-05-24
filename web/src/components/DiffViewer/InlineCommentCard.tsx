import { useState } from "react";
import { Pencil, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import type { ReviewComment } from "@/types";
import { InlineCommentForm } from "./InlineCommentForm";
import { InlineCommentFrame } from "./InlineCommentFrame";

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

  const actions = (
    <>
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
    </>
  );

  return (
    <InlineCommentFrame side={side} line={line} isDraft={isDraft} isStale={isStale} actions={actions}>
      {isEditing ? (
        <InlineCommentForm
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
    </InlineCommentFrame>
  );
}
