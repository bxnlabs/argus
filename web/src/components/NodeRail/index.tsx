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
  tooltipSide: "right" | "bottom";
}) {
  const attention = node.summary?.attention ?? 0;
  const busy = node.summary?.busy ?? 0;
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          type="button"
          data-testid={`node-tile-${node.id}`}
          data-online={node.online}
          onClick={onSelect}
          className={cn(
            "relative mx-auto flex h-10 w-10 items-center justify-center rounded-lg text-sm font-semibold transition-colors",
            active ? "bg-accent text-accent-foreground" : "text-muted-foreground hover:bg-accent/50",
            !node.online && "opacity-50 ring-1 ring-red-500/50",
          )}
        >
          {active && (
            <span aria-hidden className="bg-primary absolute left-[-6px] top-1 h-8 w-1 rounded-full" />
          )}
          {initial(node.name)}
          {node.online && attention > 0 && (
            <span
              data-testid={`node-attention-${node.id}`}
              className="absolute -right-1 -top-1 flex h-4 min-w-4 items-center justify-center rounded-full bg-blue-500 px-1 text-[10px] font-bold text-white"
            >
              {attention}
            </span>
          )}
          {node.online && attention === 0 && busy > 0 && (
            <span
              aria-hidden
              data-testid={`node-busy-${node.id}`}
              className="bg-green-500 animate-pulse-green absolute -right-0.5 -top-0.5 h-2.5 w-2.5 rounded-full"
            />
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
 * Always-visible rail of node tiles. Renders nothing when there is only the
 * local node (single-node users see the unchanged UI). `orientation="horizontal"`
 * is used by the mobile drawer.
 */
export function NodeRail({ orientation = "vertical" }: { orientation?: "vertical" | "horizontal" }) {
  const { nodes, activeNodeId, setActiveNode } = useNodeContext();
  const [manageOpen, setManageOpen] = useState(false);

  if (nodes.length < 2) return null;

  const horizontal = orientation === "horizontal";
  return (
    <>
      <div
        data-testid="node-rail"
        className={cn(
          "bg-sidebar-background flex gap-2",
          horizontal
            ? "w-full flex-row items-center overflow-x-auto border-b px-2 py-2"
            : "h-full w-12 flex-shrink-0 flex-col items-stretch border-r py-3",
        )}
      >
        {nodes.map((n) => (
          <NodeTile
            key={n.id}
            node={n}
            active={n.id === activeNodeId}
            onSelect={() => setActiveNode(n.id)}
            tooltipSide={horizontal ? "bottom" : "right"}
          />
        ))}
        <button
          type="button"
          aria-label="Manage nodes"
          onClick={() => setManageOpen(true)}
          className="text-muted-foreground hover:bg-accent/50 mx-auto flex h-10 w-10 items-center justify-center rounded-lg"
        >
          <Plus className="h-4 w-4" />
        </button>
      </div>
      <ManageNodesDialog open={manageOpen} onClose={() => setManageOpen(false)} />
    </>
  );
}
