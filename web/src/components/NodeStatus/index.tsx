import { cn } from "@/lib/utils";
import { useNodeContext } from "@/contexts/NodeContext";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import type { NodeWithStatus } from "@/types";

interface Status {
  label: string;
  className: string;
}

// Connection status only. Online wins; an unsettled first poll reads as
// Connecting; a settled failure reads as Offline. Offline is muted rather than
// red — it recedes, matching the rail's "offline never alarms" treatment.
function statusOf(node: NodeWithStatus): Status {
  if (node.online) return { label: "Online", className: "text-green-500" };
  if (node.pending) return { label: "Connecting…", className: "text-amber-500" };
  return { label: "Offline", className: "text-muted-foreground" };
}

/**
 * Compact `name · Status` line under the `argus` wordmark. Clicking it toggles
 * the node rail via `onToggleRail` (the caller already hides it when the sidebar
 * is collapsed). With no active node — an empty or errored registry — it falls
 * back to a plain "Manage nodes" toggle so the rail (and the add-node entry
 * point inside it) stays reachable instead of being orphaned.
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
        className="hover:bg-accent/50 flex w-full items-center rounded-md px-2 py-1 text-sm transition-colors"
      >
        <span className="text-muted-foreground">Manage nodes</span>
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
          className="hover:bg-accent/50 flex w-full items-center gap-1.5 rounded-md px-2 py-1 text-sm transition-colors"
        >
          <span className="text-foreground min-w-0 truncate">{activeNode.name}</span>
          <span className="text-muted-foreground flex-shrink-0">·</span>
          <span className={cn("flex-shrink-0 font-medium", status.className)}>
            {status.label}
          </span>
        </button>
      </TooltipTrigger>
      <TooltipContent side="bottom">
        {activeNode.name} · {status.label}
      </TooltipContent>
    </Tooltip>
  );
}
