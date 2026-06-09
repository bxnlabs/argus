import { cn } from "@/lib/utils";
import { nodeStatus } from "@/components/NodeStatus/status";
import type { NodeWithStatus } from "@/types";

// Each node gets a stable accent color derived from its id, so a node keeps the
// same identity color everywhere it appears (switcher, rail, panel). Mid
// saturation / lightness keeps white monograms legible on the dark sidebar.
export function nodeAccentColor(id: string): string {
  let hash = 0;
  for (let i = 0; i < id.length; i++) hash = (hash * 31 + id.charCodeAt(i)) >>> 0;
  return `hsl(${hash % 360} 52% 45%)`;
}

// The compact node monogram: the first two characters of the name, with only
// the first capitalized (Slack style — "Gp", not "GP"). Shared by the NodeAvatar
// tile and the rail's NodeTile so both read the same.
export function nodeInitials(name: string): string {
  const s = name.trim().slice(0, 2);
  if (!s) return "?";
  return s[0].toUpperCase() + s.slice(1).toLowerCase();
}

/**
 * Rounded-square monogram tile in the node's derived accent color, optionally
 * carrying a connection-status presence dot on its corner (Slack/Discord style).
 * The shared identity unit for the NodeStatus switcher, the mobile node panel,
 * and anywhere else a node needs a compact avatar.
 */
export function NodeAvatar({
  node,
  size = 32,
  showStatus = true,
  className,
}: {
  node: NodeWithStatus;
  size?: number;
  showStatus?: boolean;
  className?: string;
}) {
  const status = nodeStatus(node);
  const dot = Math.round(size * 0.34);
  return (
    <span
      className={cn(
        "relative flex flex-shrink-0 items-center justify-center rounded-md font-semibold text-white",
        className,
      )}
      style={{
        width: size,
        height: size,
        backgroundColor: nodeAccentColor(node.id),
        fontSize: Math.round(size * 0.44),
      }}
    >
      {nodeInitials(node.name)}
      {showStatus && (
        <span
          aria-hidden
          data-testid="node-avatar-dot"
          className={cn(
            "border-sidebar-background absolute -right-0.5 -bottom-0.5 rounded-full border-2",
            status.dotClassName,
          )}
          style={{ width: dot, height: dot }}
        />
      )}
    </span>
  );
}
