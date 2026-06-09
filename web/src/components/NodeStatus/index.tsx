import { Plus } from "lucide-react";
import { cn } from "@/lib/utils";
import { useNodeContext } from "@/contexts/NodeContext";
import { NodeAvatar } from "@/components/NodeAvatar";
import { nodeStatus, sourceLabel } from "./status";

// Shared card chrome: a sidebar header control (shadcn TeamSwitcher pattern).
// A prominent outline (lighter than the default border on this near-black
// sidebar) makes it read as a distinct, tappable control without a chevron — the
// affordance that also holds up on touch where there's no hover.
function cardClass(open: boolean): string {
  return cn(
    "group flex w-full items-center gap-2.5 rounded-lg border px-2 py-1.5 text-left transition-colors",
    open
      ? "border-[hsl(0_0%_34%)] bg-accent"
      : "border-[hsl(0_0%_26%)] bg-accent/30 hover:bg-accent/60",
  );
}

/**
 * Node switcher under the `argus` wordmark, styled after shadcn's sidebar
 * TeamSwitcher: the active node's accent-colored monogram tile (with a status
 * presence dot), its name and `source · status`, and a ChevronsUpDown glyph.
 * Clicking toggles the node rail via `onToggleRail` (the caller hides it when
 * the sidebar is collapsed). With no active node — an empty or errored registry
 * — it falls back to a "Manage nodes" control so the rail stays reachable.
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
        aria-label="Add node — toggle node rail"
        aria-expanded={railOpen}
        aria-controls={railOpen ? "node-rail" : undefined}
        data-testid="node-status"
        data-state={railOpen ? "open" : "closed"}
        onClick={onToggleRail}
        className={cardClass(railOpen)}
      >
        <span className="border-border bg-accent flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-md border">
          <Plus className="text-muted-foreground h-4 w-4" />
        </span>
        <span className="min-w-0 flex-1">
          <span className="text-foreground block truncate text-sm font-medium leading-tight">
            Add node
          </span>
          <span className="text-muted-foreground block truncate text-xs leading-tight">
            No active node
          </span>
        </span>
      </button>
    );
  }

  const status = nodeStatus(activeNode);
  return (
    <button
      type="button"
      aria-label={`${activeNode.name} · ${status.label} — toggle node rail`}
      aria-expanded={railOpen}
      aria-controls={railOpen ? "node-rail" : undefined}
      data-testid="node-status"
      data-state={railOpen ? "open" : "closed"}
      onClick={onToggleRail}
      className={cardClass(railOpen)}
    >
      <NodeAvatar node={activeNode} size={32} />
      <span className="min-w-0 flex-1">
        <span className="text-foreground block truncate text-sm font-semibold leading-tight">
          {activeNode.name}
        </span>
        <span className="text-muted-foreground block truncate text-xs leading-tight">
          {sourceLabel(activeNode)} · {status.label}
        </span>
      </span>
    </button>
  );
}
