import { describe, it, expect, vi } from "vitest";
import type { NodeWithStatus } from "@/types";
import { buildNodeSwitchBindings, MAX_NODE_CHORDS } from "./nodeShortcuts";

/** Minimal NodeWithStatus factory — only id/name matter to the helper. */
function node(id: string, name: string): NodeWithStatus {
  return {
    id,
    name,
    url: "",
    source: "manual",
    self: false,
    summary: null,
    online: true,
    pending: false,
  };
}

describe("buildNodeSwitchBindings", () => {
  it("returns an empty map when there is nothing to switch between", () => {
    const setActiveNode = vi.fn();
    expect(buildNodeSwitchBindings([], setActiveNode)).toEqual({});
    expect(buildNodeSwitchBindings([node("a", "Alpha")], setActiveNode)).toEqual(
      {},
    );
  });

  it("maps nodes to digit keys in order, collapsed under one label", () => {
    const setActiveNode = vi.fn();
    const bindings = buildNodeSwitchBindings(
      [node("a", "Alpha"), node("b", "Bravo"), node("c", "Charlie")],
      setActiveNode,
    );

    expect(Object.keys(bindings)).toEqual(["1", "2", "3"]);
    // Names are dropped: every chord shares one generic label + collapse token
    // so the hint overlay shows a single "1–3 Switch node" row.
    for (const key of ["1", "2", "3"]) {
      expect(bindings[key].label).toBe("Switch node");
      expect(bindings[key].collapse).toBe("node-switch");
    }

    bindings["1"].run!();
    expect(setActiveNode).toHaveBeenCalledTimes(1);
    expect(setActiveNode).toHaveBeenLastCalledWith("a");

    bindings["2"].run!();
    expect(setActiveNode).toHaveBeenCalledTimes(2);
    expect(setActiveNode).toHaveBeenLastCalledWith("b");

    bindings["3"].run!();
    expect(setActiveNode).toHaveBeenCalledTimes(3);
    expect(setActiveNode).toHaveBeenLastCalledWith("c");
  });

  it(`caps at the first ${MAX_NODE_CHORDS} nodes (rest are mouse-only)`, () => {
    const setActiveNode = vi.fn();
    const many = Array.from({ length: 11 }, (_, i) =>
      node(`id-${i}`, `Node ${i}`),
    );

    const bindings = buildNodeSwitchBindings(many, setActiveNode);

    expect(Object.keys(bindings)).toEqual([
      "1", "2", "3", "4", "5", "6", "7", "8", "9",
    ]);
    expect(bindings["10"]).toBeUndefined();
  });
});
