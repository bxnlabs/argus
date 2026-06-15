import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { ShortcutHintOverlay } from "./index";
import type { ChordMap, ChordPending } from "@/hooks/useKeyboardChords";

const bindings: ChordMap = {
  g: {
    label: "Git",
    children: {
      h: { label: "History" },
      c: { label: "Compare" },
    },
  },
  n: { label: "New session" },
  "?": { label: "Help" },
};

const noop = () => {};

describe("ShortcutHintOverlay", () => {
  it("renders nothing when pending is null and helpOpen is false", () => {
    const { container } = render(
      <ShortcutHintOverlay
        pending={null}
        bindings={bindings}
        leaderLabel="⌘ ;"
        helpOpen={false}
        onHelpOpenChange={noop}
      />,
    );
    expect(container.firstChild).toBeNull();
  });

  it("renders a row per entry with its key and label when at top-level pending", () => {
    const pending: ChordPending = { level: bindings, path: [] };
    render(
      <ShortcutHintOverlay
        pending={pending}
        bindings={bindings}
        leaderLabel="⌘ ;"
        helpOpen={false}
        onHelpOpenChange={noop}
      />,
    );

    // Heading shows the leader label (appears in the <p> heading)
    expect(screen.getAllByText("⌘ ;").length).toBeGreaterThan(0);

    // Each top-level entry's label is rendered
    expect(screen.getByText("Git")).toBeTruthy();
    expect(screen.getByText("New session")).toBeTruthy();
    expect(screen.getByText("Help")).toBeTruthy();

    // Keys rendered as kbd chips
    expect(screen.getByText("g")).toBeTruthy();
    expect(screen.getByText("n")).toBeTruthy();
    expect(screen.getByText("?")).toBeTruthy();

    // Git has children → sub-chord chevron present
    expect(screen.getAllByText("›").length).toBeGreaterThan(0);
  });

  it("collapses bindings sharing a collapse token into one ranged row", () => {
    const withNodes: ChordMap = {
      "1": { label: "Switch node", collapse: "node-switch" },
      "2": { label: "Switch node", collapse: "node-switch" },
      "3": { label: "Switch node", collapse: "node-switch" },
      // A non-collapsed sibling (unique label — these tests don't auto-clean
      // between renders, so it must not collide with another test's bindings).
      t: { label: "Terminal" },
    };
    const pending: ChordPending = { level: withNodes, path: [] };
    render(
      <ShortcutHintOverlay
        pending={pending}
        bindings={withNodes}
        leaderLabel="⌘ ;"
        helpOpen={false}
        onHelpOpenChange={noop}
      />,
    );

    // One collapsed row spanning the digit range, no per-node names/keys.
    expect(screen.getByText("1–3")).toBeTruthy();
    expect(screen.getByText("Switch node")).toBeTruthy();
    expect(screen.queryByText("2")).toBeNull();
    expect(screen.queryByText("3")).toBeNull();

    // Non-collapsed siblings still render normally.
    expect(screen.getByText("Terminal")).toBeTruthy();
  });

  it("renders breadcrumb heading when path is ['g']", () => {
    const pending: ChordPending = {
      level: bindings.g.children!,
      path: ["g"],
    };
    render(
      <ShortcutHintOverlay
        pending={pending}
        bindings={bindings}
        leaderLabel="⌘ ;"
        helpOpen={false}
        onHelpOpenChange={noop}
      />,
    );

    // The heading <p> should contain "Git ›" (breadcrumb from path ["g"])
    expect(screen.getByText("Git ›")).toBeTruthy();

    // Sub-chord entries rendered
    expect(screen.getByText("History")).toBeTruthy();
    expect(screen.getByText("Compare")).toBeTruthy();
  });
});
