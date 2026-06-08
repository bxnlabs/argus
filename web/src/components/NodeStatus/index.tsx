import { cn } from "@/lib/utils";
import { useNodeContext } from "@/contexts/NodeContext";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import type { NodeWithStatus } from "@/types";

interface Status {
  label: string;
  dotClassName: string;
}

// Connection status, conveyed by the dot's color. Online wins; an unsettled
// first poll reads as Connecting; a settled failure reads as Offline. Offline is
// muted rather than red — it recedes, matching the rail's "offline never alarms"
// treatment.
function statusOf(node: NodeWithStatus): Status {
  if (node.online) return { label: "Online", dotClassName: "bg-green-500" };
  if (node.pending) return { label: "Connecting…", dotClassName: "bg-amber-500" };
  return { label: "Offline", dotClassName: "bg-muted-foreground" };
}

// The name reads as a hyperlink so it's obvious the row opens the rail. Matches
// the `link` button variant (text-primary + hover underline); group-hover keeps
// the underline in sync when the pointer is anywhere on the row.
const nameLinkClass = "text-primary underline-offset-4 group-hover:underline";

/**
 * Compact `‹dot› ‹node name›` line under the `argus` wordmark: a colored status
 * dot (Online green, Connecting amber, Offline muted) and the active node's name
 * styled as a hyperlink. Clicking the row toggles the node rail via
 * `onToggleRail` (the caller hides it when the sidebar is collapsed). With no
 * active node — an empty or errored registry — it falls back to a "Manage nodes"
 * link so the rail (and the add-node entry point inside it) stays reachable.
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
        className="group flex w-full items-center rounded-md px-2 py-1 text-sm"
      >
        <span className={nameLinkClass}>Manage nodes</span>
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
          className="group flex w-full items-center gap-2 rounded-md px-2 py-1 text-sm"
        >
          <span
            data-testid="node-status-dot"
            aria-hidden
            className={cn("h-2 w-2 flex-shrink-0 rounded-full", status.dotClassName)}
          />
          <span className={cn("min-w-0 truncate", nameLinkClass)}>
            {activeNode.name}
          </span>
        </button>
      </TooltipTrigger>
      <TooltipContent side="bottom">
        {activeNode.name} · {status.label}
      </TooltipContent>
    </Tooltip>
  );
}
