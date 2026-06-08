import { useState } from "react";
import { Check, ChevronLeft, Plus } from "lucide-react";
import { Sheet, SheetContent, SheetTitle } from "@/components/ui/sheet";
import { useNodeContext } from "@/contexts/NodeContext";
import { ManageNodesDialog } from "@/components/ManageNodesDialog";
import { NodeAvatar } from "@/components/NodeAvatar";
import { nodeStatus, sourceLabel } from "@/components/NodeStatus/status";
import { cn } from "@/lib/utils";

/**
 * Mobile node switcher. Unlike the desktop {@link NodeRail} (a 56px icon strip
 * that pushes the sidebar), this slides a full-width panel in from the left that
 * completely overlays the sidebar drawer — Slack's workspace-switcher pattern.
 * Full rows give real touch targets and room for each node's name and status.
 *
 * Picking a node switches and closes (revealing the sidebar); the back chevron
 * or tapping the dimmed area also closes. The desktop rail is untouched.
 */
export function MobileNodePanel({
  open,
  onClose,
}: {
  open: boolean;
  onClose: () => void;
}) {
  const { nodes, activeNodeId, setActiveNode } = useNodeContext();
  const [manageOpen, setManageOpen] = useState(false);

  return (
    <Sheet open={open} onOpenChange={(o) => !o && onClose()}>
      <SheetContent
        side="left"
        hideCloseButton
        dismissOnOverlayClick
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
            return (
              <button
                key={n.id}
                type="button"
                data-testid={`node-row-${n.id}`}
                aria-current={active}
                onClick={() => {
                  setActiveNode(n.id);
                  onClose();
                }}
                className={cn(
                  "flex items-center gap-3 rounded-lg px-2.5 py-2 text-left transition-colors",
                  active ? "bg-accent" : "hover:bg-accent/50",
                )}
              >
                <NodeAvatar node={n} size={36} />
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
              </button>
            );
          })}
        </div>

        {/* Manage entry point */}
        <div className="border-border mt-auto border-t p-2 pb-[max(0.5rem,env(safe-area-inset-bottom))]">
          <button
            type="button"
            onClick={() => setManageOpen(true)}
            className="hover:bg-accent/50 flex w-full items-center gap-3 rounded-lg px-3 py-2.5 transition-colors"
          >
            <Plus className="text-muted-foreground h-[18px] w-[18px] flex-shrink-0" />
            <span className="text-muted-foreground text-[15px]">Manage nodes…</span>
          </button>
        </div>

        <ManageNodesDialog open={manageOpen} onClose={() => setManageOpen(false)} />
      </SheetContent>
    </Sheet>
  );
}
