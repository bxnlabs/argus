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
