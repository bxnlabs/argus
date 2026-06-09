import { useState } from "react";
import { cn } from "@/lib/utils";
import { useNodeContext } from "@/contexts/NodeContext";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import {
  ContextMenu, ContextMenuContent, ContextMenuItem, ContextMenuTrigger,
} from "@/components/ui/context-menu";
import { Plus, Pencil, Trash2 } from "lucide-react";
import type { NodeWithStatus } from "@/types";
import { ManageNodesDialog } from "@/components/ManageNodesDialog";
import { useDeleteNode } from "@/data/nodes/queries";
import { nodeAccentColor, nodeInitials } from "@/components/NodeAvatar";
import { UnreadBadge } from "./UnreadBadge";

function NodeTile({
  node, active, onSelect, tooltipSide, onEdit, onDelete,
}: {
  node: NodeWithStatus;
  active: boolean;
  onSelect: () => void;
  tooltipSide: "right" | "left";
  onEdit: (node: NodeWithStatus) => void;
  onDelete: (node: NodeWithStatus) => void;
}) {
  const attention = node.summary?.attention ?? 0;
  const busy = node.summary?.busy ?? 0;
  // The selected node conveys its state purely through the border colour, so it
  // never shows the unread badge or the working ring (you're already looking at
  // it). Working takes effect only on the *other* tiles.
  const working = !active && node.online && busy > 0;
  // Only manually-added (Custom) nodes can be edited/removed; the local node and
  // Tailscale-discovered peers aren't editable, so they get no menu.
  const editable = node.source === "manual";

  const button = (
    <button
      type="button"
      data-testid={`node-tile-${node.id}`}
      data-online={node.online}
      onClick={onSelect}
      // Non-editable tiles have no menu; swallow the contextmenu event so a
      // right-click doesn't pop the native browser menu over a node tile.
      onContextMenu={editable ? undefined : (e) => e.preventDefault()}
      style={{ backgroundColor: nodeAccentColor(node.id) }}
      className={cn(
        "relative mx-auto flex h-10 w-10 items-center justify-center rounded-lg border-[3px] font-mono text-lg font-bold leading-none text-white transition-[border-color,opacity,filter]",
        // The node's derived accent color is its identity (same tile as the
        // switcher avatar). Active is called out by the ring; inactive tiles
        // brighten on hover.
        active ? "border-primary" : "border-transparent hover:brightness-110",
        // Offline recedes rather than alarms: the colored tile simply dims so
        // a down node is the quietest tile in the rail, never the loudest.
        !node.online && !active && "opacity-40",
        working && "node-working",
      )}
    >
      {nodeInitials(node.name)}
      {/* Folded-corner (dog-ear) cue: marks a Custom tile as carrying a
          right-click menu. The wrapper extends over the 3px border and clips to
          the tile's rounded corner so the fold sits flush in the bottom-right.
          The flap is a *light* triangle — accent tiles are a uniform mid-dark
          (hsl 52% 45%), so white reliably contrasts where a shadow would vanish;
          the up-left drop-shadow casts a crease that lifts the flap off the
          face. Decorative — the menu lives on contextmenu. */}
      {editable && (
        <span
          aria-hidden="true"
          data-testid={`node-dogear-${node.id}`}
          className="pointer-events-none absolute -inset-[3px] overflow-hidden rounded-lg"
        >
          <span className="absolute bottom-0 right-0 h-3.5 w-3.5 bg-gradient-to-br from-white/95 to-white/70 [clip-path:polygon(100%_0,100%_100%,0_100%)] [filter:drop-shadow(-1px_-1px_1px_rgba(0,0,0,0.55))]" />
        </span>
      )}
      {!active && node.online && attention > 0 && (
        <UnreadBadge count={attention} data-testid={`node-attention-${node.id}`} />
      )}
    </button>
  );

  const tile = (
    <Tooltip>
      <TooltipTrigger asChild>
        {editable ? <ContextMenuTrigger asChild>{button}</ContextMenuTrigger> : button}
      </TooltipTrigger>
      <TooltipContent side={tooltipSide}>{node.name}</TooltipContent>
    </Tooltip>
  );

  if (!editable) return tile;

  return (
    <ContextMenu>
      {tile}
      <ContextMenuContent>
        <ContextMenuItem onSelect={() => onEdit(node)}>
          <Pencil className="mr-2 h-3.5 w-3.5" />
          Edit
        </ContextMenuItem>
        <ContextMenuItem
          className="text-destructive focus:text-destructive"
          onSelect={() => onDelete(node)}
        >
          <Trash2 className="mr-2 h-3.5 w-3.5" />
          Delete
        </ContextMenuItem>
      </ContextMenuContent>
    </ContextMenu>
  );
}

/**
 * Rail carrying the manage-nodes entry point. Visibility is controlled by the
 * caller (gated on `railOpen`, toggled from the NodeStatus snippet). Node tiles
 * only appear once there's more than the local node to switch between; with a single
 * node the rail collapses to just the manage button, so a manual (non-Tailscale)
 * user can still add their first node from the UI. The rail is always a vertical
 * strip; `side` only flips which edge carries the divider and which way tooltips
 * open — the mobile drawer puts it on the right. Custom tiles carry a right-click
 * menu to edit or remove the node.
 */
export function NodeRail({ side = "left" }: { side?: "left" | "right" }) {
  const { nodes, activeNodeId, setActiveNode } = useNodeContext();
  // The Configure Node dialog is shared between adding (node: null) and editing
  // (node: the Custom node). Closing only flips `open` so the prefilled content
  // doesn't flash blank during the close animation.
  const [dialog, setDialog] = useState<{ open: boolean; node: NodeWithStatus | null }>({
    open: false,
    node: null,
  });
  const deleteNode = useDeleteNode();

  const showTiles = nodes.length >= 2;
  return (
    <>
      <div
        id="node-rail"
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
              onEdit={(node) => setDialog({ open: true, node })}
              onDelete={(node) => deleteNode.mutate(node.id)}
            />
          ))}
        <button
          type="button"
          aria-label="Add node"
          onClick={() => setDialog({ open: true, node: null })}
          className="text-muted-foreground border-muted-foreground hover:border-white hover:text-white mx-auto flex h-8 w-8 items-center justify-center rounded-full border-2 transition-colors"
        >
          <Plus className="h-4 w-4" />
        </button>
      </div>
      <ManageNodesDialog
        open={dialog.open}
        node={dialog.node}
        onClose={() => setDialog((d) => ({ ...d, open: false }))}
      />
    </>
  );
}
