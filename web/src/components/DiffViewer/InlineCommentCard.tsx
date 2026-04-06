import { useState } from "react";
import { AlertTriangle, Pencil, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import type { ReviewComment } from "@/types";
import { InlineCommentForm } from "./InlineCommentForm";

interface InlineCommentCardProps {
  comment: ReviewComment;
  onDelete: (id: string) => void;
  onEdit: (id: string, body: string) => void;
}

export function InlineCommentCard({ comment, onDelete, onEdit }: InlineCommentCardProps) {
  const isDraft = !comment.submitted;
  const isStale = comment.anchorStatus === "stale";
  const isUnavailable = comment.anchorStatus === "context_unavailable";
  const [isEditing, setIsEditing] = useState(false);

  return (
    <div className="px-3 py-1.5 font-sans">
      <div
        className={cn(
          "bg-card/80 border-border/60 rounded-md border shadow-sm",
          isDraft && "border-l-2 border-l-primary",
          isStale && "border-yellow-500/50 bg-yellow-500/5",
          isUnavailable && "border-red-500/30 bg-red-500/5",
        )}
      >
        {/* Header */}
        <div className="flex items-center gap-2 px-3 pt-2 pb-1">
          <span
            className={cn(
              "inline-flex items-center rounded-full px-1.5 py-0.5 text-[10px] font-medium tracking-wide uppercase",
              isDraft
                ? "bg-primary/15 text-primary"
                : "bg-accent text-muted-foreground",
            )}
          >
            {isDraft ? "Pending" : "Submitted"}
          </span>
          {isStale && (
            <span className="flex items-center gap-1 text-xs text-yellow-500">
              <AlertTriangle className="h-3 w-3" />
              Anchor may have moved
            </span>
          )}
          {isUnavailable && (
            <span className="flex items-center gap-1 text-xs text-red-400">
              <AlertTriangle className="h-3 w-3" />
              Context unavailable
            </span>
          )}
          <span className="flex-1" />
          {!isEditing && (
            <Button
              size="icon-sm"
              variant="ghost"
              onClick={() => setIsEditing(true)}
              aria-label="Edit comment"
              className="text-muted-foreground hover:text-foreground -mr-1 h-6 w-6"
            >
              <Pencil className="h-3 w-3" />
            </Button>
          )}
          <Button
            size="icon-sm"
            variant="ghost"
            onClick={() => onDelete(comment.id)}
            aria-label="Delete comment"
            className="text-muted-foreground hover:text-destructive -mr-1 h-6 w-6"
          >
            <X className="h-3 w-3" />
          </Button>
        </div>

        {/* Body */}
        {isEditing ? (
          <div className="px-3 pt-0.5 pb-2.5">
            <InlineCommentForm
              bare
              initialBody={comment.body}
              submitLabel="Save"
              onSubmit={(body) => {
                onEdit(comment.id, body);
                setIsEditing(false);
              }}
              onCancel={() => setIsEditing(false)}
            />
          </div>
        ) : (
          <div className="px-3 pt-0.5 pb-2.5">
            <p className="text-foreground/90 whitespace-pre-wrap text-sm leading-relaxed">
              {comment.body}
            </p>
          </div>
        )}
      </div>
    </div>
  );
}
