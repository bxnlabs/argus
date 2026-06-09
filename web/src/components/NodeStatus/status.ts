import type { NodeInfo, NodeWithStatus } from "@/types";

export interface NodeStatusInfo {
  label: string;
  // Tailwind background-color class for the status dot.
  dotClassName: string;
}

// Where the node came from, for the switcher subtitle. The local node is "this
// machine"; discovered nodes are surfaced over Tailscale; everything else falls
// back to its raw source.
export function sourceLabel(node: NodeInfo): string {
  if (node.self) return "This machine";
  switch (node.source) {
    case "discovered":
      return "Tailscale";
    case "manual":
      return "Manual";
    default:
      return node.source;
  }
}

// Connection status, conveyed by the dot's color. Online wins; an unsettled
// first poll reads as Connecting; a settled failure reads as Offline. Offline is
// muted rather than red — it recedes, matching the rail's "offline never alarms"
// treatment. Shared by the NodeStatus snippet and the mobile node panel.
export function nodeStatus(node: NodeWithStatus): NodeStatusInfo {
  if (node.online) return { label: "Online", dotClassName: "bg-green-500" };
  if (node.pending) return { label: "Connecting…", dotClassName: "bg-amber-500" };
  return { label: "Offline", dotClassName: "bg-muted-foreground" };
}
