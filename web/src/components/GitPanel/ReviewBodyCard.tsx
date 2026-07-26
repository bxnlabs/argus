import { MessageSquare, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import type { ReviewBody } from "@/types";

interface ReviewBodyCardProps {
  body: ReviewBody;
  onDelete: () => void;
}

export function ReviewBodyCard({ body, onDelete }: ReviewBodyCardProps) {
  if (!body.body) return null;

  return (
    <div className="bg-card/80 border-border/60 font-sans rounded-md border shadow-sm">
      <div className="flex items-center gap-2 px-3 pt-2 pb-1">
        <MessageSquare className="text-muted-foreground h-3.5 w-3.5" />
        <span className="bg-accent text-muted-foreground inline-flex items-center rounded-full px-1.5 py-0.5 text-[10px] font-medium tracking-wide uppercase">
          {body.submitted ? "Submitted" : "Draft"}
        </span>
        <span className="flex-1" />
        <Button
          size="icon-sm"
          variant="ghost"
          onClick={onDelete}
          aria-label="Delete comment"
          className="text-muted-foreground hover:text-destructive -mr-1 h-6 w-6"
        >
          <X className="h-3 w-3" />
        </Button>
      </div>
      <div className="px-3 pt-0.5 pb-2.5">
        <p className="text-foreground/90 whitespace-pre-wrap text-sm leading-relaxed">
          {body.body}
        </p>
      </div>
    </div>
  );
}
