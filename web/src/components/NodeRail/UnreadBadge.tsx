import { Bell } from "lucide-react";
import { cn } from "@/lib/utils";

/**
 * The small blue count badge for unread ("attention") items on a node. Shared so
 * the per-node tiles (rail + mobile panel) and the collapsed-rail aggregate on
 * the switcher all read identically. Pinned to the top-right of a `relative`
 * parent.
 */
export function UnreadBadge({
  count,
  className,
  "data-testid": testId,
}: {
  count: number;
  className?: string;
  "data-testid"?: string;
}) {
  return (
    <span
      data-testid={testId}
      className={cn(
        "absolute -right-1 -top-1 z-10 flex h-4 min-w-4 items-center justify-center rounded-full bg-blue-500 px-1 text-[10px] font-bold text-white",
        className,
      )}
    >
      {count}
    </span>
  );
}

/**
 * Countless sibling of {@link UnreadBadge}, for the switcher tile: its badge
 * counts unread on *other* nodes while sitting on the *current* node's avatar,
 * and a digit there would read as this node's own count. The total goes in the
 * host's accessible label and tooltip instead.
 */
export function UnreadBell({
  className,
  "data-testid": testId,
}: {
  className?: string;
  "data-testid"?: string;
}) {
  return (
    <span
      aria-hidden="true"
      data-testid={testId}
      className={cn(
        "border-sidebar-background absolute -right-1 -top-1 z-10 flex h-4 w-4 items-center justify-center rounded-full border-[1.5px] bg-blue-500 text-white",
        className,
      )}
    >
      <Bell className="h-2.5 w-2.5" fill="currentColor" />
    </span>
  );
}
