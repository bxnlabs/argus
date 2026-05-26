import type { ReactNode } from "react";
import { AlertTriangle } from "lucide-react";
import { cn } from "@/lib/utils";
import type { DiffSide } from "@/types";

interface InlineCommentFrameProps {
  side: DiffSide;
  line: number;
  isDraft?: boolean;
  isStale?: boolean;
  /** Right-aligned header controls (edit/delete). */
  actions?: ReactNode;
  children: ReactNode;
}

/**
 * Shared chrome for inline comments — a left-border frame plus a header.
 * Used identically by the displayed card, edit mode, and the creation form
 * so all three stay visually unified.
 */
export function InlineCommentFrame({ side, line, isDraft, isStale, actions, children }: InlineCommentFrameProps) {
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
        {actions}
      </div>
      {children}
    </div>
  );
}
