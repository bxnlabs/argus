import { ChevronUp, ChevronDown } from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

interface CommentNavProps {
  currentIndex: number;
  total: number;
  onPrev: () => void;
  onNext: () => void;
  variant?: "inline" | "pill";
}

export function CommentNav({
  currentIndex,
  total,
  onPrev,
  onNext,
  variant = "inline",
}: CommentNavProps) {
  if (total === 0) return null;

  return (
    <div
      className={cn(
        "flex items-center gap-0.5",
        variant === "pill" &&
          "bg-popover/90 border-border/60 rounded-full border shadow-lg backdrop-blur-sm",
      )}
    >
      <Button
        variant="ghost"
        size="icon-sm"
        onClick={onPrev}
        disabled={currentIndex <= 0}
        aria-label="Previous comment"
        className="h-7 w-7 rounded-full"
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
        className="h-7 w-7 rounded-full"
      >
        <ChevronDown className="h-4 w-4" />
      </Button>
    </div>
  );
}
