import { ChevronRight } from "lucide-react";
import { cn } from "@/lib/utils";
import { useNodeContext } from "@/contexts/NodeContext";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { nodeStatus } from "./status";

// The name reads as a hyperlink so it's obvious the row opens the rail. Matches
// the `link` button variant (text-primary + hover underline); group-hover keeps
// the underline in sync when the pointer is anywhere on the row.
const nameLinkClass = "text-primary underline-offset-4 group-hover:underline";

// Disclosure chevron — the persistent "this opens the node rail" affordance.
// Mobile has no hover, so the cue can't rely on hover styling; the chevron
// carries it on both platforms and rotates to reflect the open state.
function DisclosureChevron({ open }: { open: boolean }) {
  return (
    <ChevronRight
      aria-hidden
      data-testid="node-status-chevron"
      className={cn(
        "text-muted-foreground ml-auto h-4 w-4 flex-shrink-0 transition-transform",
        open && "rotate-90",
      )}
    />
  );
}

/**
 * Compact `‹dot› ‹node name›` line under the `argus` wordmark: a colored status
 * dot (Online green, Connecting amber, Offline muted), the active node's name
 * styled as a hyperlink, and a disclosure chevron. Clicking the row toggles the
 * node rail via `onToggleRail` (the caller hides it when the sidebar is
 * collapsed). With no active node — an empty or errored registry — it falls back
 * to a "Manage nodes" link so the rail (and the add-node entry point inside it)
 * stays reachable.
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
        className="group flex w-full items-center gap-2 rounded-md px-2 py-1 text-sm"
      >
        <span className={nameLinkClass}>Manage nodes</span>
        <DisclosureChevron open={railOpen} />
      </button>
    );
  }

  const status = nodeStatus(activeNode);
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
          <DisclosureChevron open={railOpen} />
        </button>
      </TooltipTrigger>
      <TooltipContent side="bottom">
        {activeNode.name} · {status.label}
      </TooltipContent>
    </Tooltip>
  );
}
