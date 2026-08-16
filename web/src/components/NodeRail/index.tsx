import type { CSSProperties } from "react";
import { cn } from "@/lib/utils";
import { useNodeContext } from "@/contexts/NodeContext";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import {
  ContextMenu, ContextMenuContent, ContextMenuItem, ContextMenuTrigger,
} from "@/components/ui/context-menu";
import { Plus, Pencil, Trash2 } from "lucide-react";
import type { NodeWithStatus } from "@/types";
import { nodeAccentColor, nodeInitials } from "@/components/NodeAvatar";
import { UnreadBadge } from "./UnreadBadge";
import { useNodeManagement } from "./useNodeManagement";

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
  // Every online node with running sessions gets the ring, the current one
  // included: currency lives on the pill now, so the tile's border is free and
  // "is this node working?" is worth answering for the node you're standing on.
  const working = node.online && busy > 0;
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
      style={
        {
          backgroundColor: nodeAccentColor(node.id),
          // Expose the tile's 3px border so the working ring (.node-working::before)
          // offsets past it and sits flush outside the tile, matching the
          // borderless mobile avatar. Keep in sync with border-[3px] below.
          "--node-working-border": "3px",
        } as CSSProperties
      }
      className={cn(
        // The node's derived accent color is its identity (same tile as the
        // switcher avatar). Every tile carries the same 3px transparent border
        // for the working ring to offset past (--node-working-border) — currency
        // is the pill's job, so no tile spends its border on selection.
        "relative mx-auto flex h-8 w-8 items-center justify-center rounded-lg border-[3px] border-solid border-transparent text-sm font-semibold leading-none text-white transition-[opacity,filter]",
        !active && "hover:brightness-110",
        // Offline recedes rather than alarms: the colored tile simply dims so
        // a down node is the quietest tile in the rail, never the loudest. This
        // holds for the current node too — the pill still says you're on it.
        !node.online && "opacity-40",
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
      {node.online && attention > 0 && (
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

  const content = editable ? (
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
  ) : (
    tile
  );

  // Currency pill, anchored flush to the rail's leading edge — the app's own
  // outer edge, furthest from the content it selects (Discord's server rail,
  // VS Code's activity bar). White to match the session-list and view-mode
  // pills, which leaves blue to mean "unread" on its own. Flat against that
  // edge and rounded toward the tile, so it reads as growing inward off the
  // border rather than floating in the margin.
  //
  // Deliberately short — 8px, a quarter of the tile's height — but kept at the
  // full 4px thick. Length is what makes the mark loud; thickness is what makes
  // it legible, and a 2px hairline reads as an artifact of the border rather
  // than a deliberate mark. The tile is centered in the rail (see NodeRail), so
  // there are ~11px between its edge and either border and the working ring
  // spends the innermost 3px; at this size the pill still clears the ring by
  // ~4px, so a busy current node reads as a green ring *and* a white pill
  // rather than one bright smear.
  return (
    <div className="relative">
      {content}
      {active && (
        <span
          aria-hidden="true"
          data-testid={`node-pill-${node.id}`}
          className="absolute left-0 top-1/2 h-2 w-1 -translate-y-1/2 rounded-r-full bg-white"
        />
      )}
    </div>
  );
}

/**
 * Rail carrying the manage-nodes entry point. Visibility is controlled by the
 * caller (gated on `railOpen`, toggled from the NodeStatus snippet). Every node
 * gets a tile, including the current one when it's the only node — the rail
 * always shows which node you're on, matching the mobile switcher, rather than
 * collapsing to a bare manage button before any peer is discovered. The rail is
 * always a vertical strip; `side` only flips which edge carries the divider and
 * which way tooltips open — the mobile drawer puts it on the right. Custom tiles
 * carry a right-click menu to edit or remove the node.
 */
export function NodeRail({ side = "left" }: { side?: "left" | "right" }) {
  const { nodes, activeNodeId, setActiveNode } = useNodeContext();
  const { openAdd, openEdit, deleteNode, dialog } = useNodeManagement();

  // Tooltips open away from the divider edge, same as the node tiles.
  const tooltipSide = side === "right" ? "left" : "right";
  return (
    <>
      <div
        id="node-rail"
        data-testid="node-rail"
        data-side={side}
        className={cn(
          // No horizontal padding: the tiles and the add button center on the
          // rail's own axis, and the pill overlays the margin the centering
          // leaves rather than being given a lane of its own (see NodeTile).
          // Reserving one shifted every tile off-center by half its width.
          "node-rail-glass bg-sidebar-background flex h-full w-14 flex-shrink-0 flex-col items-stretch gap-3 py-3",
          side === "right" ? "border-l" : "border-r",
        )}
      >
        {nodes.map((n) => (
          <NodeTile
            key={n.id}
            node={n}
            active={n.id === activeNodeId}
            onSelect={() => setActiveNode(n.id)}
            tooltipSide={tooltipSide}
            onEdit={openEdit}
            onDelete={deleteNode}
          />
        ))}
        <Tooltip>
          <TooltipTrigger asChild>
            <button
              type="button"
              aria-label="Add node"
              onClick={openAdd}
              className="text-muted-foreground border-muted-foreground hover:border-white hover:text-white mx-auto mt-auto flex h-8 w-8 items-center justify-center rounded-full border-2 transition-colors"
            >
              <Plus className="h-4 w-4" />
            </button>
          </TooltipTrigger>
          <TooltipContent side={tooltipSide}>Add node</TooltipContent>
        </Tooltip>
      </div>
      {dialog}
    </>
  );
}
