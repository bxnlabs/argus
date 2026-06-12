import { useState } from "react";
import type { NodeWithStatus } from "@/types";
import { ManageNodesDialog } from "@/components/ManageNodesDialog";
import { useDeleteNode } from "@/data/nodes/queries";

/**
 * Shared node add/edit/delete wiring for the desktop {@link NodeRail} and the
 * {@link MobileNodePanel}: owns the Configure Node dialog state, the delete
 * mutation, and the open handlers, and returns the ready-to-render dialog. Only
 * the trigger UI differs between the two (a right-click context menu on desktop,
 * a ⋯ dropdown on mobile), so that stays in each component. Closing only flips
 * `open` so the prefilled content doesn't flash blank during the close animation.
 */
export function useNodeManagement() {
  // Shared between adding (node: null) and editing (node: the Custom node).
  const [dialog, setDialog] = useState<{ open: boolean; node: NodeWithStatus | null }>({
    open: false,
    node: null,
  });
  const deleteMutation = useDeleteNode();

  return {
    openAdd: () => setDialog({ open: true, node: null }),
    openEdit: (node: NodeWithStatus) => setDialog({ open: true, node }),
    deleteNode: (node: NodeWithStatus) => deleteMutation.mutate(node.id),
    dialog: (
      <ManageNodesDialog
        open={dialog.open}
        node={dialog.node}
        onClose={() => setDialog((d) => ({ ...d, open: false }))}
      />
    ),
  };
}
