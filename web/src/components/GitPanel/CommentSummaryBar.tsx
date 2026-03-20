import { useState, useEffect } from "react";
import { Button } from "@/components/ui/button";
import { ChevronDown, ChevronRight, MessageSquare } from "lucide-react";
import { cn } from "@/lib/utils";

interface CommentSummaryBarProps {
  pendingCount: number;
  generalComment: string;
  generalCommentSubmitted: boolean;
  onGeneralCommentChange: (body: string) => void;
  onSubmit: () => void;
  hasUnsubmitted: boolean;
}

export function CommentSummaryBar({
  pendingCount,
  generalComment,
  generalCommentSubmitted,
  onGeneralCommentChange,
  onSubmit,
  hasUnsubmitted,
}: CommentSummaryBarProps) {
  const [expanded, setExpanded] = useState(false);
  const [localGeneralComment, setLocalGeneralComment] = useState(generalComment);

  useEffect(() => {
    setLocalGeneralComment(generalComment);
  }, [generalComment]);

  return (
    <div className="border-border bg-muted/30 border-t">
      <div className="flex items-center gap-2 px-3 py-2">
        <button
          onClick={() => setExpanded(!expanded)}
          className="text-muted-foreground hover:text-foreground flex items-center gap-1 text-xs"
        >
          {expanded ? (
            <ChevronDown className="h-3.5 w-3.5" />
          ) : (
            <ChevronRight className="h-3.5 w-3.5" />
          )}
          <MessageSquare className="h-3.5 w-3.5" />
          General feedback
        </button>

        {pendingCount > 0 && (
          <span className="text-muted-foreground text-xs">
            {pendingCount} pending comment{pendingCount !== 1 ? "s" : ""}
          </span>
        )}

        <div className="ml-auto">
          <Button
            size="sm"
            onClick={onSubmit}
            disabled={!hasUnsubmitted}
          >
            Submit comments
          </Button>
        </div>
      </div>

      {expanded && (
        <div className="px-3 pb-3">
          <textarea
            value={localGeneralComment}
            onChange={(e) => setLocalGeneralComment(e.target.value)}
            onBlur={() => {
              if (localGeneralComment !== generalComment) {
                onGeneralCommentChange(localGeneralComment);
              }
            }}
            placeholder="General feedback..."
            rows={3}
            className={cn(
              "bg-background border-border w-full resize-y rounded border px-3 py-2 text-sm focus:outline-none focus:ring-1 focus:ring-blue-500",
              generalCommentSubmitted && "border-blue-500/30",
            )}
          />
        </div>
      )}
    </div>
  );
}
