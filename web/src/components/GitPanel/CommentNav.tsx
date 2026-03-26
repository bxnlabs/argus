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

  const isPill = variant === "pill";

  return (
    <div
      className={cn(
        "flex items-center",
        isPill
          ? "bg-popover/90 border-border/60 gap-1 rounded-full border px-1 py-0.5 shadow-lg backdrop-blur-sm"
          : "gap-0.5",
      )}
    >
      <Button
        variant="ghost"
        size="icon-sm"
        onClick={onPrev}
        disabled={currentIndex <= 0}
        aria-label="Previous comment"
        className={cn("rounded-full", isPill ? "h-9 w-9" : "h-7 w-7")}
      >
        <ChevronUp className={cn(isPill ? "h-5 w-5" : "h-4 w-4")} />
      </Button>
      <span className={cn(
        "text-muted-foreground min-w-[3ch] text-center tabular-nums",
        isPill ? "text-sm" : "text-xs",
      )}>
        {currentIndex >= 0 ? currentIndex + 1 : "\u2013"}/{total}
      </span>
      <Button
        variant="ghost"
        size="icon-sm"
        onClick={onNext}
        disabled={currentIndex >= total - 1}
        aria-label="Next comment"
        className={cn("rounded-full", isPill ? "h-9 w-9" : "h-7 w-7")}
      >
        <ChevronDown className={cn(isPill ? "h-5 w-5" : "h-4 w-4")} />
      </Button>
    </div>
  );
}
