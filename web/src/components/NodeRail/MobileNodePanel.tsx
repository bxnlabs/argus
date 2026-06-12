import { Check, ChevronLeft, Plus, Ellipsis, Pencil, Trash2 } from "lucide-react";
import { Sheet, SheetContent, SheetTitle } from "@/components/ui/sheet";
import {
  DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { useNodeContext } from "@/contexts/NodeContext";
import { NodeAvatar } from "@/components/NodeAvatar";
import { UnreadBadge } from "./UnreadBadge";
import { useNodeManagement } from "./useNodeManagement";
import { nodeStatus, sourceLabel } from "@/components/NodeStatus/status";
import { cn } from "@/lib/utils";

/**
 * Mobile node switcher. Unlike the desktop {@link NodeRail} (a 56px icon strip
 * that pushes the sidebar), this slides a full-width panel in from the left that
 * completely overlays the sidebar drawer — Slack's workspace-switcher pattern.
 * Full rows give real touch targets and room for each node's name and status.
 *
 * Picking a node switches the active node but keeps the panel open (switch
 * freely; the check moves). Two distinct exits: the back chevron steps back one
 * level to the sidebar ({@link onClose}); tapping the dimmed area (or Escape)
 * dismisses the whole drawer stack back to the terminal ({@link onDismiss}) —
 * the dimmed region reads as "outside everything", so one tap should get you
 * there. The desktop rail is untouched. Custom rows carry a ⋯ menu to rename or
 * remove the node.
 */
export function MobileNodePanel({
  open,
  onClose,
  onDismiss,
}: {
  open: boolean;
  onClose: () => void;
  onDismiss: () => void;
}) {
  const { nodes, activeNodeId, setActiveNode } = useNodeContext();
  const { openAdd, openEdit, deleteNode, dialog } = useNodeManagement();

  // Radix fires onOpenChange only for its own dismiss triggers (overlay tap,
  // Escape) — never for an external `open`-prop change — so the chevron's
  // onClose path stays one-level while tap-outside drives the full dismiss.
  return (
    <Sheet open={open} onOpenChange={(o) => !o && onDismiss()}>
      <SheetContent
        side="left"
        hideCloseButton
        dismissOnOverlayClick
        transparentOverlay
        className="bg-sidebar-background w-72"
      >
        {/* Header: back chevron + title */}
        <div className="flex items-center gap-2 px-3 py-3 pt-[max(0.75rem,env(safe-area-inset-top))]">
          <button
            type="button"
            aria-label="Back to sidebar"
            onClick={onClose}
            className="hover:bg-accent/50 flex h-8 w-8 items-center justify-center rounded-md transition-colors"
          >
            <ChevronLeft className="h-5 w-5" />
          </button>
          <SheetTitle className="text-lg font-bold tracking-wide">Nodes</SheetTitle>
        </div>

        {/* Node rows */}
        <div className="flex flex-col gap-0.5 px-2 py-1">
          {nodes.map((n) => {
            const status = nodeStatus(n);
            const active = n.id === activeNodeId;
            const attention = n.summary?.attention ?? 0;
            // Only Custom (manual) nodes can be renamed/removed.
            const editable = n.source === "manual";
            return (
              <div
                key={n.id}
                role="button"
                tabIndex={0}
                data-testid={`node-row-${n.id}`}
                aria-current={active}
                onClick={() => setActiveNode(n.id)}
                onKeyDown={(e) => {
                  if (e.key === "Enter" || e.key === " ") {
                    e.preventDefault();
                    setActiveNode(n.id);
                  }
                }}
                className={cn(
                  "flex cursor-pointer items-center gap-3 rounded-lg px-2.5 py-2 text-left transition-colors",
                  active ? "bg-accent" : "hover:bg-accent/50",
                )}
              >
                <span className="relative flex-shrink-0">
                  <NodeAvatar node={n} size={36} />
                  {!active && n.online && attention > 0 && (
                    <UnreadBadge count={attention} data-testid={`node-row-attention-${n.id}`} />
                  )}
                </span>
                <div className="min-w-0 flex-1">
                  <div
                    className={cn(
                      "text-foreground truncate text-[15px]",
                      active && "font-medium",
                    )}
                  >
                    {n.name}
                  </div>
                  <div className="text-muted-foreground truncate text-xs">
                    {sourceLabel(n)} · {status.label}
                  </div>
                </div>
                {active && (
                  <Check className="text-primary h-[18px] w-[18px] flex-shrink-0" />
                )}
                {editable && (
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <button
                        type="button"
                        aria-label={`Actions for ${n.name}`}
                        onClick={(e) => e.stopPropagation()}
                        className="text-muted-foreground hover:text-foreground flex-shrink-0 rounded-md p-1.5"
                      >
                        <Ellipsis className="h-4 w-4" />
                      </button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      <DropdownMenuItem
                        onClick={(e) => {
                          e.stopPropagation();
                          openEdit(n);
                        }}
                      >
                        <Pencil className="mr-2 h-3.5 w-3.5" />
                        Edit
                      </DropdownMenuItem>
                      <DropdownMenuItem
                        variant="destructive"
                        onClick={(e) => {
                          e.stopPropagation();
                          deleteNode(n);
                        }}
                      >
                        <Trash2 className="mr-2 h-3.5 w-3.5" />
                        Delete
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                )}
              </div>
            );
          })}
        </div>

        {/* Add entry point */}
        <div className="border-border mt-auto border-t p-2 pb-[max(0.5rem,env(safe-area-inset-bottom))]">
          <button
            type="button"
            onClick={openAdd}
            className="hover:bg-accent/50 flex w-full items-center gap-3 rounded-lg px-3 py-3 text-base transition-colors"
          >
            <Plus className="h-5 w-5 flex-shrink-0" />
            <span>Add node</span>
          </button>
        </div>

        {dialog}
      </SheetContent>
    </Sheet>
  );
}
