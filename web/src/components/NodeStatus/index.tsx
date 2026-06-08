import { cn } from "@/lib/utils";
import { useNodeContext } from "@/contexts/NodeContext";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import type { NodeWithStatus } from "@/types";

interface Status {
  label: string;
  dot: string;
  border: string;
}

// Connection status as a colored dot. Online wins; an unsettled first poll reads
// as Connecting; a settled failure reads as Offline. Offline is muted rather than
// red — it recedes, matching the rail's "offline never alarms" treatment. The
// pill border tracks the dot so the whole chip carries the status colour.
function statusOf(node: NodeWithStatus): Status {
  if (node.online)
    return { label: "Online", dot: "bg-green-500", border: "border-green-500" };
  if (node.pending)
    return { label: "Connecting…", dot: "bg-amber-500", border: "border-amber-500" };
  return {
    label: "Offline",
    dot: "bg-muted-foreground",
    border: "border-muted-foreground",
  };
}

/**
 * Compact status pill under the `argus` wordmark: a status dot beside the active
 * node's name in a transparent chip whose border matches the dot. Clicking it
 * toggles the node rail via `onToggleRail` (the caller hides it when the sidebar
 * is collapsed). With no active node — an empty or errored registry — it falls
 * back to a "Manage nodes" chip so the rail (and the add-node entry point inside
 * it) stays reachable instead of being orphaned.
 */
export function NodeStatus({
  railOpen,
  onToggleRail,
}: {
  railOpen: boolean;
  onToggleRail: () => void;
}) {
  const { activeNode } = useNodeContext();

  if (!activeNode) {
    return (
      <button
        type="button"
        aria-label="Manage nodes — toggle node rail"
        aria-expanded={railOpen}
        aria-controls={railOpen ? "node-rail" : undefined}
        data-testid="node-status"
        onClick={onToggleRail}
        className="border-muted-foreground text-muted-foreground hover:bg-accent/50 inline-flex max-w-full items-center rounded-full border bg-transparent px-2 py-0.5 text-sm transition-colors"
      >
        Manage nodes
      </button>
    );
  }

  const status = statusOf(activeNode);
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          type="button"
          aria-label={`${activeNode.name} · ${status.label} — toggle node rail`}
          aria-expanded={railOpen}
          aria-controls={railOpen ? "node-rail" : undefined}
          data-testid="node-status"
          onClick={onToggleRail}
          className={cn(
            "hover:bg-accent/50 inline-flex max-w-full items-center gap-1.5 rounded-full border bg-transparent px-2 py-0.5 text-sm transition-colors",
            status.border,
          )}
        >
          <span
            data-testid="node-status-dot"
            className={cn("h-2 w-2 flex-shrink-0 rounded-full", status.dot)}
          />
          <span className="text-foreground min-w-0 truncate">{activeNode.name}</span>
        </button>
      </TooltipTrigger>
      <TooltipContent side="bottom">
        {activeNode.name} · {status.label}
      </TooltipContent>
    </Tooltip>
  );
}
