import { X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import type { InlineComment } from "@/types";

interface InlineCommentCardProps {
  comment: InlineComment;
  onDelete: (id: string) => void;
}

export function InlineCommentCard({ comment, onDelete }: InlineCommentCardProps) {
  return (
    <div
      className={cn(
        "border-border bg-muted/20 border-t px-3 py-2",
        comment.submitted && "border-l-2 border-l-blue-500/50",
      )}
    >
      <div className="flex items-start gap-2">
        <p className="flex-1 whitespace-pre-wrap text-sm">{comment.body}</p>
        <Button
          size="icon-sm"
          variant="ghost"
          onClick={() => onDelete(comment.id)}
          aria-label="Delete comment"
          className="text-muted-foreground hover:text-foreground flex-shrink-0"
        >
          <X className="h-3.5 w-3.5" />
        </Button>
      </div>
      {comment.submitted && (
        <span className="text-muted-foreground mt-1 block text-xs">Submitted</span>
      )}
    </div>
  );
}
