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
 * Compact `name · Status` line under the `argus` wordmark. Renders only when a
 * node is active (the caller already hides it when the sidebar is collapsed).
 * Clicking it toggles the node rail via `onToggleRail`.
 */
export function NodeStatus({ onToggleRail }: { onToggleRail: () => void }) {
  const { activeNode } = useNodeContext();
  if (!activeNode) return null;

  const status = statusOf(activeNode);
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          type="button"
          aria-label={`${activeNode.name} · ${status.label} — toggle node rail`}
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
