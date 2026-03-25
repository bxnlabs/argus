import { ChevronUp, ChevronDown } from "lucide-react";
import { Button } from "@/components/ui/button";

interface CommentNavProps {
  currentIndex: number;
  total: number;
  onPrev: () => void;
  onNext: () => void;
}

export function CommentNav({ currentIndex, total, onPrev, onNext }: CommentNavProps) {
  if (total === 0) return null;

  return (
    <div className="flex items-center gap-0.5">
      <Button
        variant="ghost"
        size="icon-sm"
        onClick={onPrev}
        disabled={currentIndex <= 0}
        aria-label="Previous comment"
        className="h-7 w-7"
      >
        <ChevronUp className="h-4 w-4" />
      </Button>
      <span className="text-muted-foreground min-w-[3ch] text-center text-xs tabular-nums">
        {currentIndex >= 0 ? currentIndex + 1 : "\u2013"}/{total}
      </span>
      <Button
        variant="ghost"
        size="icon-sm"
        onClick={onNext}
        disabled={currentIndex >= total - 1}
        aria-label="Next comment"
        className="h-7 w-7"
      >
        <ChevronDown className="h-4 w-4" />
      </Button>
    </div>
  );
}
