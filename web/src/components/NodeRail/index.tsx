import { useState } from "react";
import { cn } from "@/lib/utils";
import { useNodeContext } from "@/contexts/NodeContext";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { Plus } from "lucide-react";
import type { NodeWithStatus } from "@/types";
import { ManageNodesDialog } from "@/components/ManageNodesDialog";

function initial(name: string): string {
  return (name.trim()[0] ?? "?").toUpperCase();
}

function NodeTile({
  node, active, onSelect, tooltipSide,
}: {
  node: NodeWithStatus;
  active: boolean;
  onSelect: () => void;
  tooltipSide: "right" | "left";
}) {
  const attention = node.summary?.attention ?? 0;
  const busy = node.summary?.busy ?? 0;
  // The selected node conveys its state purely through the border colour, so it
  // never shows the unread badge or the working ring (you're already looking at
  // it). Working takes effect only on the *other* tiles.
  const working = !active && node.online && busy > 0;
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          type="button"
          data-testid={`node-tile-${node.id}`}
          data-online={node.online}
          onClick={onSelect}
          className={cn(
            "relative mx-auto flex h-10 w-10 items-center justify-center rounded-lg border-[3px] font-mono text-lg font-bold leading-none transition-colors",
            active
              ? "border-primary bg-accent text-accent-foreground"
              : "border-transparent text-foreground hover:bg-accent/50",
            // Offline recedes rather than alarms: no border (an idle online node
            // has none either), just a dimmed, desaturated letter so a down node
            // is the quietest tile in the rail, never the loudest.
            !node.online && !active && "text-muted-foreground opacity-40",
            working && "node-working",
          )}
        >
          {initial(node.name)}
          {!active && node.online && attention > 0 && (
            <span
              data-testid={`node-attention-${node.id}`}
              className="absolute -right-1 -top-1 z-10 flex h-4 min-w-4 items-center justify-center rounded-full bg-blue-500 px-1 text-[10px] font-bold text-white"
            >
              {attention}
            </span>
          )}
        </button>
      </TooltipTrigger>
      <TooltipContent side={tooltipSide}>
        {node.name}
        {!node.online && " · offline"}
      </TooltipContent>
    </Tooltip>
  );
}

/**
 * Rail carrying the manage-nodes entry point. Visibility is controlled by the
 * caller (gated on `railOpen`, toggled from the NodeStatus snippet). Node tiles
 * only appear once there's more than the local node to switch between; with a single
 * node the rail collapses to just the manage button, so a manual (non-Tailscale)
 * user can still add their first node from the UI. The rail is always a vertical
 * strip; `side` only flips which edge carries the divider and which way tooltips
 * open — the mobile drawer puts it on the right.
 */
export function NodeRail({ side = "left" }: { side?: "left" | "right" }) {
  const { nodes, activeNodeId, setActiveNode } = useNodeContext();
  const [manageOpen, setManageOpen] = useState(false);

  const showTiles = nodes.length >= 2;
  return (
    <>
      <div
        data-testid="node-rail"
        className={cn(
          "bg-sidebar-background flex h-full w-14 flex-shrink-0 flex-col items-stretch gap-3 py-3",
          side === "right" ? "border-l" : "border-r",
        )}
      >
        {showTiles &&
          nodes.map((n) => (
            <NodeTile
              key={n.id}
              node={n}
              active={n.id === activeNodeId}
              onSelect={() => setActiveNode(n.id)}
              tooltipSide={side === "right" ? "left" : "right"}
            />
          ))}
        <button
          type="button"
          aria-label="Manage nodes"
          onClick={() => setManageOpen(true)}
          className="text-muted-foreground border-muted-foreground hover:border-green-500 hover:text-green-500 mx-auto flex h-8 w-8 items-center justify-center rounded-full border-2 transition-colors"
        >
          <Plus className="h-4 w-4" />
        </button>
      </div>
      <ManageNodesDialog open={manageOpen} onClose={() => setManageOpen(false)} />
    </>
  );
}
