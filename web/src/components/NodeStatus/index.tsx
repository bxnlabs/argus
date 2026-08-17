import { Plus } from "lucide-react";
import { useNodeContext } from "@/contexts/NodeContext";
import { NodeAvatar } from "@/components/NodeAvatar";
import { UnreadBell } from "@/components/NodeRail/UnreadBadge";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { nodeStatus } from "./status";

/**
 * Node switcher beside the `argus` wordmark: just the active node's accent tile
 * (with its status presence dot). Clicking toggles the node rail (desktop) /
 * panel (mobile) via `onToggleRail`. While the rail is collapsed the tile carries
 * a top-left bell whenever unread is waiting on the *other* online nodes — what
 * you can't see with the rail closed; once it's open those counts live on the
 * individual rail tiles instead. The bell carries no number: a digit on this tile
 * would read as the *current* node's unread, so the total goes in the label and
 * tooltip. With no active node (empty/errored registry) it falls back to a Plus
 * tile so the rail stays reachable.
 */
export function NodeStatus({
  railOpen,
  onToggleRail,
}: {
  railOpen: boolean;
  onToggleRail: () => void;
}) {
  const { activeNode, nodes } = useNodeContext();

  const sharedProps = {
    type: "button" as const,
    "aria-expanded": railOpen,
    "aria-controls": railOpen ? "node-rail" : undefined,
    "data-testid": "node-status",
    "data-state": railOpen ? "open" : "closed",
    onClick: onToggleRail,
    className:
      "relative flex-shrink-0 rounded-md transition-[filter] hover:brightness-110",
  };

  if (!activeNode) {
    return (
      <Tooltip>
        <TooltipTrigger asChild>
          <button {...sharedProps} aria-label="Add node — toggle node rail">
            <span className="border-border bg-accent flex h-8 w-8 items-center justify-center rounded-md border">
              <Plus className="text-muted-foreground h-4 w-4" />
            </span>
          </button>
        </TooltipTrigger>
        <TooltipContent side="bottom">Add node</TooltipContent>
      </Tooltip>
    );
  }

  const status = nodeStatus(activeNode);
  // The *other* online nodes with unread — the active node is excluded, since
  // you're already looking at it.
  const others = nodes.filter(
    (n) => n.id !== activeNode.id && n.online && (n.summary?.attention ?? 0) > 0,
  );
  const otherUnread = others.reduce((sum, n) => sum + (n.summary?.attention ?? 0), 0);
  // The bell carries no digits, so the count lives here — the only place it can
  // say whose it is.
  const unreadLabel =
    otherUnread > 0
      ? ` · ${otherUnread} unread on ${others.length === 1 ? "another node" : "other nodes"}`
      : "";

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          {...sharedProps}
          aria-label={`${activeNode.name} · ${status.label}${unreadLabel} — toggle node rail`}
        >
          <NodeAvatar node={activeNode} size={32} />
          {!railOpen && otherUnread > 0 && (
            <UnreadBell data-testid="node-status-attention" />
          )}
        </button>
      </TooltipTrigger>
      <TooltipContent side="bottom">
        {activeNode.name} · {status.label}
        {unreadLabel}
      </TooltipContent>
    </Tooltip>
  );
}
