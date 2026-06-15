import type { ChordMap } from "@/hooks/useKeyboardChords";
import type { NodeWithStatus } from "@/types";

/** Highest node reachable by a number chord; beyond this requires the mouse. */
export const MAX_NODE_CHORDS = 9;

/**
 * Build the `1`–`9` chord bindings that switch the active node, in rail order
 * (`1` = first tile). Returns an empty map when there is nothing to switch
 * between (a single node or none), so the chord/help overlay never advertises a
 * no-op. Nodes past the 9th get no binding — they are reachable only by
 * clicking the rail tile. Each binding mirrors a tile click: `setActiveNode(id)`.
 *
 * Every binding shares a `collapse` token so the hint overlay renders them as a
 * single "1–9 Switch node" row rather than one named row per node.
 */
export function buildNodeSwitchBindings(
  nodes: NodeWithStatus[],
  setActiveNode: (id: string) => void,
): ChordMap {
  if (nodes.length < 2) return {};

  const bindings: ChordMap = {};
  nodes.slice(0, MAX_NODE_CHORDS).forEach((node, i) => {
    bindings[String(i + 1)] = {
      label: "Switch node",
      collapse: "node-switch",
      run: () => setActiveNode(node.id),
    };
  });
  return bindings;
}
