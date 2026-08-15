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
 * The countless sibling of {@link UnreadBadge}, for the one place a number would
 * lie: the switcher tile, whose badge sums the unread waiting on *other* nodes
 * while sitting on the *current* node's avatar. A digit there reads as "this
 * node has 5" — so the bell says only "something's waiting in the rail" and
 * leaves the counting to the rail's own tiles.
 *
 * Same 16px circle in the same corner as the count badge, so the two are
 * interchangeable in the slot. The glyph is filled rather than stroked (a
 * hairline bell turns to mush at 10px) and the badge carries a surface-colored
 * ring, since a silhouette needs a clean edge where digits don't. Decorative —
 * the count it stands for belongs in the host's accessible label.
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
